package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeneratePlan handles one-shot AI workout plan generation. It renders the
// generate_plan prompt with the request parameters, calls the Gemini API with a
// structured JSON schema, and validates all returned exercise names against the
// known exercise list. It retries up to 3 times if the AI returns invalid names.
func GeneratePlan(c *fiber.Ctx) error {
	var req GeneratePlanReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	prompt, err := renderPrompt("generate_plan", map[string]any{
		"Days":         req.Days,
		"Level":        req.Level,
		"Goal":         req.Goal,
		"ExerciseList": allowedExerciseNames,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load prompt"})
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to initialise AI client"})
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")
	model.ResponseMIMEType = "application/json"
	model.ResponseSchema = planSchema()

	var rawJSON string
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil || len(resp.Candidates) == 0 {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "AI failed to generate plan"})
		}

		rawJSON = string(resp.Candidates[0].Content.Parts[0].(genai.Text))

		if bad := invalidExercisesInJSON(rawJSON); len(bad) == 0 {
			if resp.UsageMetadata != nil {
				fmt.Printf("Token usage (plan): input=%d output=%d total=%d\n",
					resp.UsageMetadata.PromptTokenCount,
					resp.UsageMetadata.CandidatesTokenCount,
					resp.UsageMetadata.TotalTokenCount)
			}
			break
		} else {
			fmt.Printf("Plan attempt %d: invalid exercises: %v\n", attempt, bad)
			if attempt == 3 {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "AI could not generate a valid plan"})
			}
			prompt += fmt.Sprintf(
				"\n\nSYSTEM ERROR: Your last response contained invalid exercises: %s. "+
					"You MUST only use exact names from the allowed list. Try again.",
				strings.Join(bad, ", "),
			)
		}
	}

	c.Set("Content-Type", "application/json")
	return c.SendString(rawJSON)
}

// planSchema returns the Gemini response schema for a workout plan.
func planSchema() *genai.Schema {
	return &genai.Schema{
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
	}
}

// invalidExercisesInJSON unmarshals a raw plan JSON string and returns any
// exercise names that are not in the allowed exercise map.
func invalidExercisesInJSON(rawJSON string) []string {
	var plan struct {
		Exercises []struct {
			Name string `json:"name"`
		} `json:"exercises"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &plan); err != nil {
		return nil
	}
	var bad []string
	for _, ex := range plan.Exercises {
		if !allowedExerciseMap[ex.Name] {
			bad = append(bad, ex.Name)
		}
	}
	return bad
}
