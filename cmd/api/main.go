package main

import (
	"log"
	"os"

	"fitness-backend/internal/app"
	"fitness-backend/internal/db"
	"fitness-backend/internal/utils"
)

func main() {
	utils.LoadEnv()

	db.Connect()
	defer db.Close()
	db.Migrate()

	server := app.Setup()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s...\n", port)
	log.Fatal(server.Listen(":" + port))
}
