package utils

import (
	"log"

	"github.com/joho/godotenv"
)

// loads the .env file for local development. In production, environment
// variables are expected to be provided by the host (Docker, cloud platform, etc.),
// so a missing .env file is not treated as an error.
func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found. Relying on system environment variables.")
	}
}

// LoadTestEnv searches parent directories for a .env.test file, allowing tests
// in deeply nested packages to locate the file from the project root.
func LoadTestEnv() {
	paths := []string{
		".env.test",
		"../.env.test",
		"../../.env.test",
		"../../../.env.test",
	}
	for _, path := range paths {
		if err := godotenv.Load(path); err == nil {
			return
		}
	}
}
