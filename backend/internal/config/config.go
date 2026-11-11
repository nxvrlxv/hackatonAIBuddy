package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	//OllamaURL   string
	//EmbedModel  string
	//LLMModel    string
	Port      string
	JWTSecret string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("empty database url")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("Can't load jwt secret, empty")
	}

	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
		JWTSecret:   jwtSecret,
	}, nil
}
