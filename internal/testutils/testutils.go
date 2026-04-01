package testutils

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"fitness-backend/internal/db"
	"fitness-backend/internal/utils"

	"github.com/gofiber/fiber/v2"
)

// dbOnce ensures the database connection, migrations, and unique test email are
// initialised exactly once per test binary. Each package compiles to its own
// binary, so parallel package runs each get an isolated email namespace.
var (
	dbOnce    sync.Once
	testEmail string
)

// SetupTestApp prepares a clean test environment: it connects to the test DB
// (once per binary), wipes all data belonging to this package's test user,
// seeds a fresh test user, and returns a bare Fiber app with the user's ID.
// Routes must be registered by the caller.
//
// Requires TEST_DATABASE_URL in .env.test. Tests are skipped if it is not set.
func SetupTestApp(t *testing.T) (*fiber.App, string) {
	t.Helper()

	utils.LoadTestEnv()

	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	dbOnce.Do(func() {
		os.Setenv("DATABASE_URL", os.Getenv("TEST_DATABASE_URL"))
		os.Setenv("JWT_SECRET", "test_secret_123")
		db.Connect()
		db.Migrate()
		// Unique email per binary prevents parallel packages from conflicting
		// on the same test@example.com row in a shared test database.
		testEmail = fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	})

	// Wipe only this package's test user data so parallel packages don't
	// interfere. FK cascades handle plans and history automatically.
	for _, stmt := range []string{
		"DELETE FROM history WHERE user_id IN (SELECT id FROM users WHERE email = $1)",
		"DELETE FROM plans   WHERE user_id IN (SELECT id FROM users WHERE email = $1)",
		"DELETE FROM users   WHERE email = $1",
	} {
		if _, err := db.DB.Exec(context.Background(), stmt, testEmail); err != nil {
			t.Fatalf("Failed to clean test data (%s): %v", stmt, err)
		}
	}

	var userID string
	err := db.DB.QueryRow(context.Background(),
		`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`,
		testEmail, "Test User",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to seed test user: %v", err)
	}

	return fiber.New(), userID
}
