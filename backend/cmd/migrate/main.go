package main

import (
	"context"
	"log"
	"os"

	"github.com/dt-camera/backend/internal/database"
)

func main() {
	db, err := database.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("database open failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := database.Ping(ctx, db); err != nil {
		log.Fatalf("database health check failed: %v", err)
	}

	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	log.Printf("migrations applied")
}
