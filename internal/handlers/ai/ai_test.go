package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// --- LoadExercises ---

func TestLoadExercises_Success(t *testing.T) {
	mockData := `[
		{"id": 1, "displayName": "Barbell Bench Press"},
		{"id": 2, "name": "Squat"},
		{"id": 3, "displayName": "Deadlift"}
	]`

	f, _ := os.CreateTemp("", "exercises_*.json")
	defer os.Remove(f.Name())
	f.WriteString(mockData)
	f.Close()

	allowedExerciseMap = make(map[string]bool)
	allowedExerciseNames = ""

	err := LoadExercises(f.Name())
	assert.NoError(t, err)
	assert.True(t, allowedExerciseMap["Barbell Bench Press"])
	assert.True(t, allowedExerciseMap["Squat"])
	assert.True(t, allowedExerciseMap["Deadlift"])
	assert.False(t, allowedExerciseMap["Bicep Curl"], "Unlisted exercise should not be present")
}

func TestLoadExercises_FileNotFound(t *testing.T) {
	err := LoadExercises("/nonexistent/path/exercises.json")
	assert.Error(t, err)
}

func TestLoadExercises_InvalidJSON(t *testing.T) {
	f, _ := os.CreateTemp("", "exercises_bad_*.json")
	defer os.Remove(f.Name())
	f.WriteString(`{not valid json}`)
	f.Close()

	err := LoadExercises(f.Name())
	assert.Error(t, err)
}

// --- renderPrompt ---

func TestRenderPrompt_MissingKey(t *testing.T) {
	promptCache = make(map[string]string)
	_, err := renderPrompt("nonexistent_key", nil)
	assert.Error(t, err)
}

func TestRenderPrompt_RendersTemplate(t *testing.T) {
	promptCache = map[string]string{
		"test_prompt": "Hello {{.Name}}, you have {{.Days}} days.",
	}
	result, err := renderPrompt("test_prompt", map[string]any{"Name": "Alice", "Days": 7})
	assert.NoError(t, err)
	assert.Equal(t, "Hello Alice, you have 7 days.", result)
}

func TestRenderPrompt_InvalidTemplate(t *testing.T) {
	promptCache = map[string]string{
		"bad_template": "Hello {{.Name",
	}
	_, err := renderPrompt("bad_template", nil)
	assert.Error(t, err)
}

// --- trimExerciseData ---

func TestTrimExerciseData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []map[string]interface{}
	}{
		{
			name: "nested wgerData with sets array",
			input: `[{
				"wgerData": {"displayName": "Barbell Lunges Standing"},
				"sets": [{"reps": "10"}, {"reps": "10"}, {"reps": "10"}]
			}]`,
			expected: []map[string]interface{}{
				{"name": "Barbell Lunges Standing", "sets": float64(3), "target_reps": "10"},
			},
		},
		{
			name: "flat AI-generated structure",
			input: `[{"name": "Pushups", "sets": 4, "target_reps": "15-20"}]`,
			expected: []map[string]interface{}{
				{"name": "Pushups", "sets": float64(4), "target_reps": "15-20"},
			},
		},
		{
			name: "legacy wger_search_query fallback",
			input: `[{"wger_search_query": "Deadlift", "sets": 5}]`,
			expected: []map[string]interface{}{
				{"name": "Deadlift", "sets": float64(5)},
			},
		},
		{
			name:     "invalid input returns raw fallback",
			input:    `{"junk": "data"}`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw interface{}
			json.Unmarshal([]byte(tt.input), &raw)

			result := trimExerciseData(raw)

			if tt.expected == nil {
				assert.NotNil(t, result, "Invalid input should return the raw value, not nil")
				return
			}

			trimmed, ok := result.([]map[string]interface{})
			if !assert.True(t, ok, "Expected []map[string]interface{}") {
				return
			}

			assert.Len(t, trimmed, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, fmt.Sprintf("%v", exp["name"]), fmt.Sprintf("%v", trimmed[i]["name"]))
				assert.Equal(t, fmt.Sprintf("%v", exp["sets"]), fmt.Sprintf("%v", trimmed[i]["sets"]))
				assert.Equal(t, fmt.Sprintf("%v", exp["target_reps"]), fmt.Sprintf("%v", trimmed[i]["target_reps"]))
			}
		})
	}
}

// --- invalidExercisesInJSON ---

func TestInvalidExercisesInJSON(t *testing.T) {
	allowedExerciseMap = map[string]bool{
		"Bench Press": true,
		"Squat":       true,
	}

	t.Run("all valid", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"exercises": []any{
				map[string]any{"name": "Bench Press"},
				map[string]any{"name": "Squat"},
			},
		})
		assert.Empty(t, invalidExercisesInJSON(string(raw)))
	})

	t.Run("contains invalid exercise", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"exercises": []any{
				map[string]any{"name": "Bench Press"},
				map[string]any{"name": "Made Up Exercise"},
			},
		})
		bad := invalidExercisesInJSON(string(raw))
		assert.Equal(t, []string{"Made Up Exercise"}, bad)
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		assert.Nil(t, invalidExercisesInJSON("{bad json}"))
	})
}

// --- GeneratePlan handler ---

func TestGeneratePlan_BadRequest(t *testing.T) {
	app := fiber.New()
	app.Post("/plan", GeneratePlan)

	req := httptest.NewRequest("POST", "/plan", bytes.NewBufferString("{bad json}"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	assert.Equal(t, 400, resp.StatusCode)
}
