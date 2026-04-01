package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ChatCoach handles multi-turn fitness coaching conversations. It uses Gemini
// function calling with FunctionCallingAny mode to guarantee a structured
// response via the deliver_final_response tool on every turn.
//
// Flow per request:
//  1. Run the intent check to decide whether to inject the exercise list.
//  2. Build the system prompt and attach tools.
//  3. Replay the trimmed conversation history into the session.
//  4. Send the new message and enter the tool loop.
//  5. Dispatch DB tool calls as needed, validate any plan data, then return.
func ChatCoach(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var body struct {
		History    []ChatMessage `json:"history"`
		NewMessage string        `json:"newMessage"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to initialise AI client"})
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")

	systemPrompt, err := buildSystemPrompt(ctx, client, body.NewMessage)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to build system prompt"})
	}

	model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))
	model.Tools = []*genai.Tool{chatTools()}
	model.ToolConfig = &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingAny,
		},
	}

	session := model.StartChat()
	session.History = buildHistory(body.History)

	resp, err := session.SendMessage(ctx, genai.Text(body.NewMessage))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "AI generation failed"})
	}

	result, err := runToolLoop(ctx, session, resp, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if result != nil {
		c.Set("Content-Type", "application/json")
		return c.SendString(string(result))
	}

	// Fallback if the tool loop exhausts all hops without a final response.
	fallback, _ := json.Marshal(map[string]string{
		"response_type": "chat",
		"text":          "I needed a little too much time to think about that. Could you try rephrasing?",
	})
	c.Set("Content-Type", "application/json")
	return c.SendString(string(fallback))
}

// buildSystemPrompt renders the base system prompt and conditionally appends
// the exercise list if the intent check determines the user wants to create or
// modify a plan.
func buildSystemPrompt(ctx context.Context, client *genai.Client, userMessage string) (string, error) {
	base, err := renderPrompt("chat_system_base", map[string]any{
		"Date": time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return "", err
	}

	if checkIntent(ctx, client, userMessage) {
		fmt.Println("Intent: plan creation/modification detected — injecting exercise list.")
		rules, err := renderPrompt("chat_system_exercises", map[string]any{
			"ExerciseList": allowedExerciseNames,
		})
		if err == nil {
			return base + "\n\n" + rules, nil
		}
	} else {
		fmt.Println("Intent: general chat — skipping exercise list.")
	}

	return base, nil
}

// checkIntent makes a small, cheap Gemini call to decide whether the user's
// message requires the exercise list to be injected. Returns true only when the
// user wants to create new exercises or modify which exercises are in a plan.
// Defaults to true on any error so the AI is never missing context it might need.
func checkIntent(ctx context.Context, client *genai.Client, userMessage string) bool {
	model := client.GenerativeModel("gemini-2.5-flash")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"needs_exercises": {Type: genai.TypeBoolean},
		},
	}

	prompt, err := renderPrompt("intent_check", map[string]any{"Message": userMessage})
	if err != nil {
		return true
	}

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil || len(resp.Candidates) == 0 {
		return true
	}

	var result struct {
		NeedsExercises bool `json:"needs_exercises"`
	}
	json.Unmarshal([]byte(string(resp.Candidates[0].Content.Parts[0].(genai.Text))), &result)
	return result.NeedsExercises
}

// buildHistory converts the client-side message slice into Gemini's history
// format, capping at the last 6 messages to limit context size. AI messages
// stored as JSON are unwrapped back to plain text, with a note appended if
// the message included a generated plan.
func buildHistory(history []ChatMessage) []*genai.Content {
	const maxHistory = 6
	start := 0
	if len(history) > maxHistory {
		start = len(history) - maxHistory
	}

	var out []*genai.Content
	for i := start; i < len(history); i++ {
		msg := history[i]

		role := "user"
		if msg.Role == "ai" {
			role = "model"
		}

		text := msg.Text
		var parsed map[string]any
		if err := json.Unmarshal([]byte(msg.Text), &parsed); err == nil {
			if textVal, ok := parsed["text"].(string); ok {
				text = textVal
				if _, hasPlan := parsed["plan_data"]; hasPlan {
					text += "\n[A workout plan was generated in this message.]"
				}
			}
		}

		out = append(out, &genai.Content{
			Role:  role,
			Parts: []genai.Part{genai.Text(text)},
		})
	}
	return out
}

// chatTools defines the three Gemini function declarations available to the
// chat model: two DB read tools and the mandatory response delivery tool.
func chatTools() *genai.Tool {
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "get_workout_history",
				Description: "Fetches the user's past workouts. Use when the user asks about their progress, a specific exercise, or a date range.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"start_date":    {Type: genai.TypeString, Description: "ISO 8601 date, e.g. 2026-01-01"},
						"end_date":      {Type: genai.TypeString, Description: "ISO 8601 date, e.g. 2026-01-31"},
						"exercise_name": {Type: genai.TypeString, Description: "Filter to a specific exercise name"},
						"limit":         {Type: genai.TypeInteger, Description: "Max records to return (1–50)"},
					},
				},
			},
			{
				Name:        "get_saved_plans",
				Description: "Fetches the user's saved workout plan templates.",
				Parameters: &genai.Schema{
					Type:       genai.TypeObject,
					Properties: map[string]*genai.Schema{},
				},
			},
			{
				Name:        "deliver_final_response",
				Description: "Delivers your final response to the user. Always call this to send your answer.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"response_type": {Type: genai.TypeString, Description: "'chat' or 'plan'"},
						"text":          {Type: genai.TypeString, Description: "Your conversational reply"},
						"plan_data": {
							Type: genai.TypeObject,
							Properties: map[string]*genai.Schema{
								"name": {Type: genai.TypeString},
								"exercises": {
									Type: genai.TypeArray,
									Items: &genai.Schema{
										Type: genai.TypeObject,
										Properties: map[string]*genai.Schema{
											"name":        {Type: genai.TypeString},
											"sets":        {Type: genai.TypeInteger},
											"target_reps": {Type: genai.TypeString},
										},
									},
								},
							},
						},
					},
					Required: []string{"response_type", "text"},
				},
			},
		},
	}
}

// runToolLoop drives the Gemini multi-turn tool call cycle. On each hop it
// dispatches DB read tools, validates exercise names in deliver_final_response
// calls, and returns the final JSON payload once the model is satisfied.
// The loop is capped at 4 hops to prevent runaway API usage.
func runToolLoop(ctx context.Context, session *genai.ChatSession, resp *genai.GenerateContentResponse, userID string) ([]byte, error) {
	current := resp
	for hop := 0; hop < 4; hop++ {
		if len(current.Candidates) == 0 {
			break
		}

		var fc *genai.FunctionCall
		for _, part := range current.Candidates[0].Content.Parts {
			if call, ok := part.(genai.FunctionCall); ok {
				fc = &call
				break
			}
		}

		// With FunctionCallingAny this should never be nil, but guard defensively.
		if fc == nil {
			break
		}

		if fc.Name == "deliver_final_response" {
			if bad := validatePlanExercises(fc.Args); len(bad) > 0 {
				fmt.Printf("Invalid exercises in response: %v\n", bad)
				errMsg := fmt.Sprintf(
					"SYSTEM ERROR: Your response contained invalid exercise names: %s. "+
						"Use only exact names from the allowed list and try again.",
					strings.Join(bad, ", "),
				)
				var err error
				current, err = session.SendMessage(ctx, genai.FunctionResponse{
					Name:     fc.Name,
					Response: map[string]any{"result": errMsg},
				})
				if err != nil {
					return nil, fmt.Errorf("AI tool processing failed")
				}
				continue
			}

			if current.UsageMetadata != nil {
				fmt.Printf("Token usage (chat): input=%d output=%d total=%d\n",
					current.UsageMetadata.PromptTokenCount,
					current.UsageMetadata.CandidatesTokenCount,
					current.UsageMetadata.TotalTokenCount)
			}

			return json.Marshal(fc.Args)
		}

		// Dispatch DB read tools and feed the result back to the model.
		result := dispatchDBTool(fc, userID)
		var err error
		current, err = session.SendMessage(ctx, genai.FunctionResponse{
			Name:     fc.Name,
			Response: map[string]any{"result": result},
		})
		if err != nil {
			return nil, fmt.Errorf("AI tool processing failed")
		}
	}
	return nil, nil
}

// validatePlanExercises extracts exercise names from a deliver_final_response
// args map and returns any that are not in the allowed exercise map.
func validatePlanExercises(args map[string]any) []string {
	planData, ok := args["plan_data"].(map[string]any)
	if !ok {
		return nil
	}
	exList, ok := planData["exercises"].([]any)
	if !ok {
		return nil
	}
	var bad []string
	for _, item := range exList {
		if exMap, ok := item.(map[string]any); ok {
			if name, ok := exMap["name"].(string); ok && !allowedExerciseMap[name] {
				bad = append(bad, name)
			}
		}
	}
	return bad
}

// dispatchDBTool executes the named DB read tool and returns the result as a
// JSON string, or a human-readable "no data" message if the result is empty.
func dispatchDBTool(fc *genai.FunctionCall, userID string) string {
	switch fc.Name {
	case "get_workout_history":
		fmt.Println("Tool call: get_workout_history")
		filter := HistoryFilter{}
		if v, ok := fc.Args["start_date"].(string); ok {
			filter.StartDate = v
		}
		if v, ok := fc.Args["end_date"].(string); ok {
			filter.EndDate = v
		}
		if v, ok := fc.Args["exercise_name"].(string); ok {
			filter.ExerciseName = v
		}
		if v, ok := fc.Args["limit"].(float64); ok {
			filter.Limit = int(v)
		}
		b, _ := json.Marshal(fetchUserHistory(userID, filter))
		if s := string(b); s != "null" && s != "[]" {
			return s
		}
		return "No workout history found for this request."

	case "get_saved_plans":
		fmt.Println("Tool call: get_saved_plans")
		b, _ := json.Marshal(fetchUserPlans(userID))
		if s := string(b); s != "null" && s != "[]" {
			return s
		}
		return "No saved plans found."

	default:
		return "Error: unknown function"
	}
}
