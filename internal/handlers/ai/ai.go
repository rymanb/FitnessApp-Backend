package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"fitness-backend/internal/db"
)

// allowedExerciseNames is the full comma-separated exercise name list injected
// into prompts when the AI needs to generate or modify exercises.
var allowedExerciseNames string

// allowedExerciseMap provides O(1) lookups for validating AI-generated exercise
// names against the known exercise list.
var allowedExerciseMap = make(map[string]bool)

// promptCache holds prompt templates loaded from the database at startup,
// keyed by their prompt name.
var promptCache = make(map[string]string)

// GeneratePlanReq is the request body for the one-shot plan generation endpoint.
type GeneratePlanReq struct {
	Level string `json:"level"`
	Goal  string `json:"goal"`
	Days  int    `json:"days"`
}

// ChatMessage represents a single turn in a conversation as sent by the client.
type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// LoadExercises reads the exercises JSON file and populates the name cache and
// validation map used by both the plan generator and the chat coach.
func LoadExercises(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var exercises []map[string]interface{}
	if err := json.Unmarshal(data, &exercises); err != nil {
		return err
	}

	var names []string
	for _, ex := range exercises {
		if name, ok := ex["displayName"].(string); ok {
			names = append(names, name)
			allowedExerciseMap[name] = true
		} else if name, ok := ex["name"].(string); ok {
			names = append(names, name)
			allowedExerciseMap[name] = true
		}
	}

	allowedExerciseNames = strings.Join(names, ", ")
	fmt.Printf("Loaded %d exercises.\n", len(names))
	return nil
}

// LoadPrompts fetches all prompt templates from the database into memory.
// Templates use Go's text/template syntax (e.g. {{.FieldName}}) and are
// rendered at request time via renderPrompt.
func LoadPrompts() error {
	if db.DB == nil {
		return fmt.Errorf("database not connected")
	}
	rows, err := db.DB.Query(context.Background(), "SELECT key, template FROM prompts")
	if err != nil {
		return fmt.Errorf("failed to query prompts: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var key, tmpl string
		if err := rows.Scan(&key, &tmpl); err != nil {
			return err
		}
		promptCache[key] = tmpl
		count++
	}
	fmt.Printf("Loaded %d prompt templates.\n", count)
	return nil
}

// renderPrompt looks up a prompt template by key and renders it with the
// provided data. Returns an error if the key is missing or the template fails.
func renderPrompt(key string, data any) (string, error) {
	tmpl, ok := promptCache[key]
	if !ok {
		return "", fmt.Errorf("prompt template not found: %s", key)
	}
	t, err := template.New(key).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse prompt %s: %w", key, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render prompt %s: %w", key, err)
	}
	return buf.String(), nil
}
