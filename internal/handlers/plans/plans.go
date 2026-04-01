package plans

import (
	"context"
	"fmt"
	"time"

	"fitness-backend/internal/db"

	"github.com/gofiber/fiber/v2"
)

// Plan mirrors the workout plan structure used by the mobile client.
// Exercises is kept as interface{} to accept the arbitrary JSONB structure
// stored on the client without requiring a rigid schema on the server.
type Plan struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Exercises interface{} `json:"exercises"`
	UpdatedAt time.Time   `json:"updatedAt"`
	IsDeleted bool        `json:"isDeleted"`
}

// GetPlans returns all plans for the authenticated user. The mobile client
// can optionally delta-sync by providing an ?after= timestamp query parameter.
func GetPlans(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	rows, err := db.DB.Query(context.Background(), `
		SELECT id, name, exercises, updated_at, is_deleted
		FROM plans
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch plans"})
	}
	defer rows.Close()

	plans := []Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Exercises, &p.UpdatedAt, &p.IsDeleted); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to parse plans"})
		}
		plans = append(plans, p)
	}

	return c.JSON(plans)
}

// SyncPlans accepts a batch of plans from the mobile client and upserts each one
// using a last-write-wins strategy: an incoming record only overwrites an existing
// one if its updated_at timestamp is newer.
func SyncPlans(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var incoming []Plan
	if err := c.BodyParser(&incoming); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	query := `
		INSERT INTO plans (id, user_id, name, exercises, updated_at, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		SET
			name       = EXCLUDED.name,
			exercises  = EXCLUDED.exercises,
			updated_at = EXCLUDED.updated_at,
			is_deleted = EXCLUDED.is_deleted
		WHERE plans.updated_at < EXCLUDED.updated_at
	`

	synced := 0
	for _, p := range incoming {
		if _, err := db.DB.Exec(context.Background(), query,
			p.ID, userID, p.Name, p.Exercises, p.UpdatedAt, p.IsDeleted,
		); err != nil {
			fmt.Printf("Failed to sync plan %s: %v\n", p.ID, err)
			continue
		}
		synced++
	}

	return c.JSON(fiber.Map{
		"message":      "sync complete",
		"items_synced": synced,
	})
}
