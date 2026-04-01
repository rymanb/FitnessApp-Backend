package db

import (
	"context"
	"fitness-backend/internal/utils" // Import our new utility
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDBLifecycle(t *testing.T) {
	utils.LoadTestEnv()

	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	os.Setenv("DATABASE_URL", os.Getenv("TEST_DATABASE_URL"))

	// Test Connection
	Connect()
	assert.NotNil(t, DB, "Database connection pool should not be nil")

	// Verify the connection is actually alive
	err := DB.Ping(context.Background())
	assert.NoError(t, err, "Should be able to ping the database successfully")

	// 3. Test Migrations
	// (Our Migrate function already knows how to find the SQL file from this folder!)
	assert.NotPanics(t, func() {
		Migrate()
	}, "Database migration should not crash")

	// 4. Test Clean Shutdown
	assert.NotPanics(t, func() {
		Close()
	}, "Closing the database connection should not crash")
}
