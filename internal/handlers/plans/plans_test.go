package plans

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

func TestPlans_Unauthorized(t *testing.T) {
	app, _ := testutils.SetupTestApp(t)
	app.Get("/api/v1/plans", middleware.Protected(), GetPlans)

	req := httptest.NewRequest("GET", "/api/v1/plans", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestGetPlans_ReturnsEmptySlice(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Get("/api/v1/plans", middleware.Protected(), GetPlans)

	req := httptest.NewRequest("GET", "/api/v1/plans", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result []Plan
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result, "Should return an empty array, not null")
	assert.Len(t, result, 0)
}

func TestGetPlans_ReturnsSavedPlans(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Get("/api/v1/plans", middleware.Protected(), GetPlans)

	db.DB.Exec(context.Background(),
		"INSERT INTO plans (id, user_id, name, exercises) VALUES ($1, $2, $3, $4)",
		"550e8400-e29b-41d4-a716-446655441111", userID, "Push Day", "[]")

	req := httptest.NewRequest("GET", "/api/v1/plans", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result []Plan
	json.NewDecoder(resp.Body).Decode(&result)
	if assert.Len(t, result, 1) {
		assert.Equal(t, "Push Day", result[0].Name)
	}
}

func TestSyncPlans_InvalidBody(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/plans/sync", middleware.Protected(), SyncPlans)

	req := httptest.NewRequest("POST", "/api/v1/plans/sync", bytes.NewBufferString("{bad json}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 400, resp.StatusCode)
}

func TestSyncPlans_ReturnsItemsSyncedCount(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/plans/sync", middleware.Protected(), SyncPlans)

	plans := []Plan{
		{ID: "550e8400-e29b-41d4-a716-000000000001", Name: "Plan A", Exercises: []interface{}{}, UpdatedAt: time.Now()},
		{ID: "550e8400-e29b-41d4-a716-000000000002", Name: "Plan B", Exercises: []interface{}{}, UpdatedAt: time.Now()},
	}
	body, _ := json.Marshal(plans)

	req := httptest.NewRequest("POST", "/api/v1/plans/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(2), result["items_synced"])
}

func TestSyncPlans_SoftDeletePropagates(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/plans/sync", middleware.Protected(), SyncPlans)

	planID := "550e8400-e29b-41d4-a716-446655440099"
	plans := []Plan{{
		ID:        planID,
		Name:      "Deleted Plan",
		Exercises: []interface{}{},
		UpdatedAt: time.Now(),
		IsDeleted: true,
	}}
	body, _ := json.Marshal(plans)

	req := httptest.NewRequest("POST", "/api/v1/plans/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+generateTestToken(userID))
	app.Test(req)

	var isDeleted bool
	db.DB.QueryRow(context.Background(), "SELECT is_deleted FROM plans WHERE id = $1", planID).Scan(&isDeleted)
	assert.True(t, isDeleted, "is_deleted flag should be persisted to the database")
}

// Verify last-write-wins: a stale incoming record must not overwrite a newer one.
func TestSyncPlans_LastWriteWins(t *testing.T) {
	app, userID := testutils.SetupTestApp(t)
	app.Post("/api/v1/plans/sync", middleware.Protected(), SyncPlans)

	planID := "550e8400-e29b-41d4-a716-446655440000"
	token := generateTestToken(userID)

	syncPlan := func(name string, updatedAt time.Time) {
		body, _ := json.Marshal([]Plan{{
			ID:        planID,
			Name:      name,
			Exercises: []interface{}{},
			UpdatedAt: updatedAt,
		}})
		req := httptest.NewRequest("POST", "/api/v1/plans/sync", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		app.Test(req)
	}

	syncPlan("New Name", time.Now())
	syncPlan("Old Name", time.Now().Add(-time.Hour))

	var name string
	db.DB.QueryRow(context.Background(), "SELECT name FROM plans WHERE id = $1", planID).Scan(&name)
	assert.Equal(t, "New Name", name, "Stale record should not overwrite the newer version")
}
