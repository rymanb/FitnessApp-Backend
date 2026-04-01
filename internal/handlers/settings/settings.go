package settings

import (
	"context"
	"time"

	"fitness-backend/internal/db"

	"github.com/gofiber/fiber/v2"
)

// SettingsData holds the user-configurable workout preferences.
type SettingsData struct {
	RestTimersEnabled   bool   `json:"restTimersEnabled"`
	SetRestSeconds      int    `json:"setRestSeconds"`
	ExerciseRestSeconds int    `json:"exerciseRestSeconds"`
	Theme               string `json:"theme"`
}

// Settings is the full payload exchanged with the client.
type Settings struct {
	Data      SettingsData `json:"data"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// GetSettings returns the stored settings for the authenticated user.
// Returns defaults if no settings have been saved yet.
func GetSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var s Settings
	err := db.DB.QueryRow(context.Background(), `
		SELECT data, updated_at FROM settings WHERE user_id = $1
	`, userID).Scan(&s.Data, &s.UpdatedAt)

	if err != nil {
		// No row yet — return defaults so the client knows the server has nothing
		return c.JSON(Settings{
			Data: SettingsData{
				RestTimersEnabled:   true,
				SetRestSeconds:      90,
				ExerciseRestSeconds: 120,
				Theme:               "dark",
			},
			UpdatedAt: time.Time{},
		})
	}

	return c.JSON(s)
}

// SyncSettings upserts the user's settings using last-write-wins: the incoming
// record only overwrites an existing one if its updated_at is newer.
func SyncSettings(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var incoming Settings
	if err := c.BodyParser(&incoming); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	_, err := db.DB.Exec(context.Background(), `
		INSERT INTO settings (user_id, data, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET
			data       = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at
		WHERE settings.updated_at < EXCLUDED.updated_at
	`, userID, incoming.Data, incoming.UpdatedAt)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to sync settings"})
	}

	return c.JSON(fiber.Map{"message": "settings synced"})
}
