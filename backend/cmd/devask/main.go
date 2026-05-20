package main

import (
	"log"
	"os"

	"backend/internal/cli"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present (gracefully ignore if absent, e.g., in production)
	if err := godotenv.Load(); err != nil {
		// Only warn if the file exists but failed to parse
		if !os.IsNotExist(err) {
			log.Printf("Warning: could not load .env file: %v", err)
		}
	}
	cli.Execute()
}
