package main

import (
	"log"

	"github.com/nxvrlxv/ai-buddy/backend/internal/migrator"
)

func main() {
	dbURL := "postgres://ai_buddy:ai_buddy@db:5432/ai_buddy?sslmode=disable"
	if err := migrator.Run(dbURL); err != nil {
		log.Fatal(err)
	}
	log.Println("migrations applied")
}
