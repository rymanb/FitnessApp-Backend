package history

import (
	"context"
	"fmt"
	"time"

	"fitness-backend/internal/db"

	"github.com/gofiber/fiber/v2"
)

// HistoryRecord mirrors the completed workout structure used by the mobile client.
type HistoryRecord struct {
	ID              string      `json:"id"`
	PlanID          string      `json:"planId"`
	PlanName        string      `json:"planName"`
	DateCompleted   time.Time   `json:"dateCompleted"`
	Exercises       interface{} `json:"exercises"`
	DurationSeconds int         `json:"durationSeconds"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	IsDeleted       bool        `json:"isDeleted"`
}

// GetHistory returns workout history for the authenticated user. An optional
// ?after=<RFC3339 timestamp> query parameter enables delta sync — only records
// updated after that time are returned.
func GetHistory(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	afterDate := c.Query("after")

	query := `
		SELECT id, plan_id, plan_name, date_completed, exercises, duration_seconds, updated_at, is_deleted
		FROM history
		WHERE user_id = $1
	`
	args := []interface{}{userID}

	if afterDate != "" {
		query += ` AND updated_at > $2`
		args = append(args, afterDate)
	}

	query += ` ORDER BY updated_at DESC`

	rows, err := db.DB.Query(context.Background(), query, args...)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch history"})
	}
	defer rows.Close()

	records := []HistoryRecord{}
	for rows.Next() {
		var h HistoryRecord
		if err := rows.Scan(&h.ID, &h.PlanID, &h.PlanName, &h.DateCompleted, &h.Exercises, &h.DurationSeconds, &h.UpdatedAt, &h.IsDeleted); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse history"})
		}
		records = append(records, h)
	}

	return c.JSON(records)
}

// SyncHistory accepts a batch of completed workout records from the mobile client
// and upserts each one using a last-write-wins strategy.
func SyncHistory(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var incoming []HistoryRecord
	if err := c.BodyParser(&incoming); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	query := `
		INSERT INTO history (id, user_id, plan_id, plan_name, date_completed, exercises, duration_seconds, updated_at, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE
		SET
			plan_id          = EXCLUDED.plan_id,
			plan_name        = EXCLUDED.plan_name,
			date_completed   = EXCLUDED.date_completed,
			exercises        = EXCLUDED.exercises,
			duration_seconds = EXCLUDED.duration_seconds,
			updated_at       = EXCLUDED.updated_at,
			is_deleted       = EXCLUDED.is_deleted
		WHERE history.updated_at < EXCLUDED.updated_at
	`

	synced := 0
	for _, r := range incoming {
		if _, err := db.DB.Exec(context.Background(), query,
			r.ID, userID, r.PlanID, r.PlanName, r.DateCompleted, r.Exercises, r.DurationSeconds, r.UpdatedAt, r.IsDeleted,
		); err != nil {
			fmt.Printf("Failed to sync history record %s: %v\n", r.ID, err)
			continue
		}
		synced++
	}

	return c.JSON(fiber.Map{
		"message":      "sync complete",
		"items_synced": synced,
	})
}
