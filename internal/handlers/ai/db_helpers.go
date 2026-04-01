package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"fitness-backend/internal/db"
)

// HistoryFilter holds optional filters for the workout history query.
type HistoryFilter struct {
	StartDate    string
	EndDate      string
	ExerciseName string
	Limit        int
}

// trimExerciseData reduces a raw exercise array (as stored in the DB) down to
// the minimal fields the AI needs: name, sets, and target_reps. This prevents
// large wgerData blobs from inflating the AI context window.
func trimExerciseData(rawExercises interface{}) interface{} {
	b, err := json.Marshal(rawExercises)
	if err != nil {
		return rawExercises
	}

	var exArray []map[string]interface{}
	if err := json.Unmarshal(b, &exArray); err != nil {
		return rawExercises
	}

	var trimmed []map[string]interface{}
	for _, exMap := range exArray {
		cleanEx := make(map[string]interface{})

		// Resolve the exercise name from whichever shape is present.
		if wgerData, ok := exMap["wgerData"].(map[string]interface{}); ok {
			if val, exists := wgerData["displayName"]; exists && val != "" {
				cleanEx["name"] = val
			} else if val, exists := wgerData["name"]; exists {
				cleanEx["name"] = val
			}
		} else if val, exists := exMap["displayName"]; exists {
			cleanEx["name"] = val
		} else if val, exists := exMap["name"]; exists {
			cleanEx["name"] = val
		} else if val, exists := exMap["wger_search_query"]; exists {
			cleanEx["name"] = val
		}

		// Resolve sets and reps from whichever shape is present.
		if setsArray, ok := exMap["sets"].([]interface{}); ok {
			cleanEx["sets"] = len(setsArray)
			if len(setsArray) > 0 {
				if firstSet, ok := setsArray[0].(map[string]interface{}); ok {
					if reps, exists := firstSet["reps"]; exists {
						cleanEx["target_reps"] = reps
					}
				}
			}
		} else {
			if val, exists := exMap["sets"]; exists {
				cleanEx["sets"] = val
			}
			if val, exists := exMap["target_reps"]; exists {
				cleanEx["target_reps"] = val
			}
		}

		trimmed = append(trimmed, cleanEx)
	}

	if len(trimmed) == 0 {
		return rawExercises
	}
	return trimmed
}

// fetchUserHistory queries the history table with optional filters and returns
// a trimmed, AI-friendly representation of each completed workout.
func fetchUserHistory(userID string, filter HistoryFilter) []map[string]any {
	query := `SELECT plan_name, date_completed, exercises FROM history WHERE user_id = $1 AND is_deleted = false`
	args := []interface{}{userID}
	n := 1

	if filter.StartDate != "" {
		n++
		query += fmt.Sprintf(` AND date_completed >= $%d`, n)
		args = append(args, filter.StartDate)
	}
	if filter.EndDate != "" {
		n++
		query += fmt.Sprintf(` AND date_completed <= $%d`, n)
		args = append(args, filter.EndDate)
	}
	if filter.ExerciseName != "" {
		n++
		query += fmt.Sprintf(` AND exercises::text ILIKE $%d`, n)
		args = append(args, "%"+filter.ExerciseName+"%")
	}

	query += ` ORDER BY date_completed DESC`

	if filter.Limit > 0 && filter.Limit <= 50 {
		n++
		query += fmt.Sprintf(` LIMIT $%d`, n)
		args = append(args, filter.Limit)
	} else {
		query += ` LIMIT 15`
	}

	rows, err := db.DB.Query(context.Background(), query, args...)
	var results []map[string]any
	if err != nil {
		fmt.Printf("DB error fetching history: %v\n", err)
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var date time.Time
		var ex interface{}
		if err := rows.Scan(&name, &date, &ex); err == nil {
			results = append(results, map[string]any{
				"plan_name":      name,
				"date_completed": date.Format("2006-01-02"),
				"exercises":      trimExerciseData(ex),
			})
		}
	}
	return results
}

// fetchUserPlans returns the user's most recently updated saved plans,
// trimmed to the fields the AI needs.
func fetchUserPlans(userID string) []map[string]any {
	rows, err := db.DB.Query(context.Background(), `
		SELECT name, exercises
		FROM plans
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY updated_at DESC
		LIMIT 10
	`, userID)

	var results []map[string]any
	if err != nil {
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var ex interface{}
		if err := rows.Scan(&name, &ex); err == nil {
			results = append(results, map[string]any{
				"name":      name,
				"exercises": trimExerciseData(ex),
			})
		}
	}
	return results
}
