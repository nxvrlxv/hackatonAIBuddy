package main

import (
	"log"
	"os"

	"github.com/nxvrlxv/ai-buddy/backend/internal/migrator"
)

func main() {
	if err := migrator.Run(os.Getenv("DATABASE_URL")); err != nil {
		log.Fatal(err)
	}
	log.Println("migrations applied (database is up to date)")
}
