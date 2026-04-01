package history

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"fitness-backend/internal/db"
	"fitness-backend/internal/middleware"
	"fitness-backend/internal/testutils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func generateTestToken(userID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := token.SignedString([]byte("test_secret_123"))
	return s
}

func insertHistory(t *testing.T, id, userID, planID, planName string, completedAt time.Time) {
	t.Helper()
	_, err := db.DB.Exec(context.Background(),
		`INSERT INTO history (id, user_id, plan_id, plan_name, date_completed, updated_at, exercises)
		 VALUES ($1, $2, $3, $4, $5, $6, '[]')`,
		id, userID, planID, planName, completedAt, completedAt,
	)
	if err != nil {
		t.Fatalf("Failed to insert history fixture: %v", err)
	}
}

func TestHistory_Unauthorized(t *testing.T) {
	app, _ := testutils.SetupTestApp(t)
	app.Get("/api/v1/history", middleware.Protected(), GetHistory)

	req := httptest.NewRequest("GET", "/api/v1/history", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetHistory_ReturnsEmptySlice(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Get("/api/v1/history", middleware.Protected(), GetHistory)

	req := httptest.NewRequest("GET", "/api/v1/history", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result []HistoryRecord
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result, "Should return an empty array, not null")
	assert.Len(t, result, 0)
}

func TestGetHistory_ReturnsAllRecords(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Get("/api/v1/history", middleware.Protected(), GetHistory)

	insertHistory(t, "550e8400-e29b-41d4-a716-000000000001", userID, "p1", "Leg Day", time.Now().Add(-2*time.Hour))
	insertHistory(t, "550e8400-e29b-41d4-a716-000000000002", userID, "p2", "Push Day", time.Now().Add(-1*time.Hour))

	req := httptest.NewRequest("GET", "/api/v1/history", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result []HistoryRecord
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Len(t, result, 2)
}

func TestGetHistory_DeltaSync(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Get("/api/v1/history", middleware.Protected(), GetHistory)

	insertHistory(t, "550e8400-e29b-41d4-a716-000000000001", userID, "p1", "Old Workout", time.Now().Add(-48*time.Hour))
	insertHistory(t, "550e8400-e29b-41d4-a716-000000000002", userID, "p2", "New Workout", time.Now().Add(-1*time.Hour))

	after := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/v1/history?after="+after, nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result []HistoryRecord
	json.NewDecoder(resp.Body).Decode(&result)
	if assert.Len(t, result, 1, "Delta sync should only return records after the cursor") {
		assert.Equal(t, "New Workout", result[0].PlanName)
	}
}

func TestSyncHistory_InvalidBody(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/history/sync", middleware.Protected(), SyncHistory)

	req := httptest.NewRequest("POST", "/api/v1/history/sync", bytes.NewBufferString("{bad json}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 400, resp.StatusCode)
}

func TestSyncHistory_PersistsRecord(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/history/sync", middleware.Protected(), SyncHistory)

	records := []HistoryRecord{{
		ID:            "550e8400-e29b-41d4-a716-446655440003",
		PlanID:        "plan-1",
		PlanName:      "Leg Day",
		DateCompleted: time.Now().Round(time.Second),
		Exercises:     []interface{}{},
		UpdatedAt:     time.Now().Round(time.Second),
	}}
	body, _ := json.Marshal(records)

	req := httptest.NewRequest("POST", "/api/v1/history/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(1), result["items_synced"])

	var count int
	db.DB.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM history WHERE id = '550e8400-e29b-41d4-a716-446655440003'",
	).Scan(&count)
	assert.Equal(t, 1, count)
}

func TestSyncHistory_SoftDeletePropagates(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/history/sync", middleware.Protected(), SyncHistory)

	recordID := "550e8400-e29b-41d4-a716-446655440099"
	records := []HistoryRecord{{
		ID:            recordID,
		PlanID:        "plan-x",
		PlanName:      "Old Session",
		DateCompleted: time.Now().Round(time.Second),
		Exercises:     []interface{}{},
		UpdatedAt:     time.Now().Round(time.Second),
		IsDeleted:     true,
	}}
	body, _ := json.Marshal(records)

	req := httptest.NewRequest("POST", "/api/v1/history/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	app.Test(req)

	var isDeleted bool
	db.DB.QueryRow(context.Background(),
		"SELECT is_deleted FROM history WHERE id = $1", recordID,
	).Scan(&isDeleted)
	assert.True(t, isDeleted)
}

// Verify last-write-wins: a stale incoming record must not overwrite a newer one.
func TestSyncHistory_LastWriteWins(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/history/sync", middleware.Protected(), SyncHistory)

	recordID := "550e8400-e29b-41d4-a716-446655440077"
	token := generateTestToken(userID)
	now := time.Now().Round(time.Second)

	syncRecord := func(name string, updatedAt time.Time) {
		body, _ := json.Marshal([]HistoryRecord{{
			ID:            recordID,
			PlanID:        "plan-1",
			PlanName:      name,
			DateCompleted: now,
			Exercises:     []interface{}{},
			UpdatedAt:     updatedAt,
		}})
		req := httptest.NewRequest("POST", "/api/v1/history/sync", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		app.Test(req)
	}

	syncRecord("New Session", now)
	syncRecord("Old Session", now.Add(-time.Hour))

	var name string
	db.DB.QueryRow(context.Background(),
		"SELECT plan_name FROM history WHERE id = $1", recordID,
	).Scan(&name)
	assert.Equal(t, "New Session", name, "Stale record should not overwrite the newer version")
}
