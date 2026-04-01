package app

import (
	"fmt"

	"fitness-backend/internal/handlers/ai"
	"fitness-backend/internal/handlers/auth"
	"fitness-backend/internal/handlers/history"
	"fitness-backend/internal/handlers/plans"
	"fitness-backend/internal/handlers/settings"
	"fitness-backend/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// Setup initializes the Fiber app, registers middleware, and mounts all routes.
func Setup() *fiber.App {
	if err := ai.LoadExercises("exercises.json"); err != nil {
		fmt.Printf("Warning: could not load AI exercise context: %v\n", err)
	}
	if err := ai.LoadPrompts(); err != nil {
		fmt.Printf("Warning: could not load AI prompts from DB: %v\n", err)
	}

	app := fiber.New(fiber.Config{
		AppName: "Fitness App Backend v1.0",
	})

	app.Use(logger.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "server is running",
		})
	})

	api := app.Group("/api/v1")

	// Public routes
	api.Post("/auth/google", auth.GoogleAuth)

	// Protected routes — require a valid JWT
	protected := api.Group("/", middleware.Protected())

	protected.Get("/plans", plans.GetPlans)
	protected.Post("/plans/sync", plans.SyncPlans)
	protected.Post("/plans/generate", ai.GeneratePlan)

	protected.Get("/history", history.GetHistory)
	protected.Post("/history/sync", history.SyncHistory)

	protected.Get("/settings", settings.GetSettings)
	protected.Post("/settings/sync", settings.SyncSettings)

	protected.Post("/chat", ai.ChatCoach)

	return app
}
