package plans

import (
	"context"
	"fmt"
	"time"

	"fitness-backend/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// SharedPlanResponse is the payload returned by GetSharedPlan.
type SharedPlanResponse struct {
	PlanName     string      `json:"planName"`
	Exercises    interface{} `json:"exercises"`
	CreatorName  string      `json:"creatorName"`
	CreatorPhoto string      `json:"creatorPhoto"`
}

// SharePlan creates a shareable snapshot of a plan owned by the authenticated user.
// Returns the share ID which the client uses to construct a deep link.
func SharePlan(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	planID := c.Params("planId")

	var p Plan
	err := db.DB.QueryRow(context.Background(), `
		SELECT id, name, exercises FROM plans
		WHERE id = $1 AND user_id = $2 AND is_deleted = false
	`, planID, userID).Scan(&p.ID, &p.Name, &p.Exercises)
	if err != nil {
		if err == pgx.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "plan not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to look up plan"})
	}

	var shareID string
	err = db.DB.QueryRow(context.Background(), `
		INSERT INTO shared_plans (owner_id, plan_name, exercises)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, p.Name, p.Exercises).Scan(&shareID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create share"})
	}

	return c.JSON(fiber.Map{"shareId": shareID})
}

// GetSharedPlan returns the plan data and creator info for a share link. Public endpoint.
func GetSharedPlan(c *fiber.Ctx) error {
	shareID := c.Params("shareId")

	var resp SharedPlanResponse
	err := db.DB.QueryRow(context.Background(), `
		SELECT sp.plan_name, sp.exercises, u.name, COALESCE(u.photo, '')
		FROM shared_plans sp
		JOIN users u ON u.id = sp.owner_id
		WHERE sp.id = $1
	`, shareID).Scan(&resp.PlanName, &resp.Exercises, &resp.CreatorName, &resp.CreatorPhoto)
	if err != nil {
		if err == pgx.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "shared plan not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch shared plan"})
	}

	return c.JSON(resp)
}

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

// ShareRedirect serves an HTML page that immediately opens the app via deep link.
// This makes the share URL a normal https:// link that is clickable in any app.
func ShareRedirect(c *fiber.Ctx) error {
	shareID := c.Params("shareId")
	deepLink := "fitness-app://share/" + shareID
	c.Set("Content-Type", "text/html")
	return c.SendString(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Opening workout plan…</title>
<style>
  body { font-family: -apple-system, sans-serif; background: #09090b; color: #a1a1aa;
         display: flex; flex-direction: column; align-items: center; justify-content: center;
         min-height: 100vh; margin: 0; text-align: center; padding: 24px; box-sizing: border-box; }
  h1 { color: #fafafa; font-size: 1.4rem; margin-bottom: 8px; }
  p  { font-size: 0.95rem; margin-bottom: 32px; }
  a  { color: #6366f1; font-size: 0.9rem; }
</style>
</head>
<body>
<h1>Opening in Fitness App…</h1>
<p>If the app doesn't open, make sure it's installed.</p>
<a href="` + deepLink + `">Tap here to open manually</a>
<script>window.location.replace("` + deepLink + `");</script>
</body>
</html>`)
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
