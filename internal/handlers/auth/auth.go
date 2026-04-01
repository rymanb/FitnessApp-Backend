package auth

import (
	"context"
	"fmt"
	"os"
	"time"

	"fitness-backend/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/api/idtoken"
)

// AuthRequest is the payload expected from the mobile client.
type AuthRequest struct {
	IDToken string `json:"idToken"`
}

// GoogleAuth verifies a Google ID token, upserts the user in the database,
// and returns a signed JWT for subsequent API requests.
func GoogleAuth(c *fiber.Ctx) error {
	var req AuthRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Verify the Google ID token cryptographically against our client ID.
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	payload, err := idtoken.Validate(context.Background(), req.IDToken, clientID)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid Google token"})
	}

	email := fmt.Sprintf("%v", payload.Claims["email"])
	name := fmt.Sprintf("%v", payload.Claims["name"])
	photo := fmt.Sprintf("%v", payload.Claims["picture"])

	// Upsert the user — update profile fields on returning users.
	var userID string
	query := `
		INSERT INTO users (email, name, photo)
		VALUES ($1, $2, $3)
		ON CONFLICT (email)
		DO UPDATE SET name = EXCLUDED.name, photo = EXCLUDED.photo
		RETURNING id
	`
	if err = db.DB.QueryRow(context.Background(), query, email, name, photo).Scan(&userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "database error"})
	}

	// Issue a short-lived JWT the mobile client will use as a Bearer token.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(72 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate token"})
	}

	return c.JSON(fiber.Map{
		"token": tokenString,
		"user": fiber.Map{
			"id":    userID,
			"email": email,
			"name":  name,
			"photo": photo,
		},
	})
}
