package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the shared connection pool used by all handlers.
var DB *pgxpool.Pool

// Connect establishes the PostgreSQL connection pool and verifies connectivity.
func Connect() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v\n", err)
	}

	fmt.Println("Connected to PostgreSQL.")
	DB = pool
}

// Migrate runs all SQL migration files in order. Each file uses IF NOT EXISTS /
// ON CONFLICT guards so it is safe to run on every startup.
func Migrate() {
	migrations := []string{"001_init.sql", "002_prompts.sql", "003_history_duration.sql", "004_settings.sql", "005_daily_token_usage.sql"}
	for _, file := range migrations {
		sqlBytes, err := findMigrationFile(file)
		if err != nil {
			log.Fatalf("Failed to read migration file %s: %v\n", file, err)
		}
		if _, err = DB.Exec(context.Background(), string(sqlBytes)); err != nil {
			log.Fatalf("Failed to execute migration %s: %v\n", file, err)
		}
	}
	fmt.Println("Database migrations applied.")
}

// findMigrationFile searches common relative paths for a migration file,
// allowing tests run from subdirectories to locate the sql/ folder.
func findMigrationFile(filename string) ([]byte, error) {
	prefixes := []string{
		"sql/migrations/",
		"../sql/migrations/",
		"../../sql/migrations/",
		"../../../sql/migrations/",
	}
	for _, prefix := range prefixes {
		data, err := os.ReadFile(prefix + filename)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("could not find %s in any expected path", filename)
}

// Close gracefully shuts down the connection pool.
func Close() {
	if DB != nil {
		DB.Close()
		fmt.Println("Database connection closed.")
	}
}
