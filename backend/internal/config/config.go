package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	OllamaURL   string
	EmbedModel  string
	LLMModel    string
	Port        string
	JWTSecret   string
	FileStore   string
}

func Load() (*Config, error) {
	if err := loadProjectEnv(); err != nil {
		return nil, err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("empty database url")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ollamaUrl := os.Getenv("OLLAMA_URL")
	if ollamaUrl == "" {
		return nil, fmt.Errorf("empty ollama url")
	}

	llm := os.Getenv("LLM_MODEL")
	if llm == "" {
		return nil, fmt.Errorf("model was not found")
	}
	fileStore := os.Getenv("KNOWLEDGE_PATH")
	if fileStore == "" {
		return nil, fmt.Errorf("store path is not defined")
	}

	embedModel := os.Getenv("EMBEDDED_MODEL")
	if embedModel == "" {
		return nil, fmt.Errorf("embedModel was not found")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("Can't load jwt secret, empty")
	}

	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
		LLMModel:    llm,
		OllamaURL:   ollamaUrl,
		JWTSecret:   jwtSecret,
		EmbedModel:  embedModel,
		FileStore:   fileStore,
	}, nil
}

// loadProjectEnv locates the repository root by backend/go.mod, starting at the
// working directory. Nested .env files are ignored. An absent root/.env is fine
// for deployments configured entirely through process environment variables.
func loadProjectEnv() error {
	directory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	for {
		marker := filepath.Join(directory, "backend", "go.mod")
		info, err := os.Stat(marker)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("locate project root: %w", err)
		}
		if err == nil && !info.IsDir() {
			filename := filepath.Join(directory, ".env")
			if err := godotenv.Load(filename); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				// Parser errors can include source lines containing secrets.
				return fmt.Errorf("cannot read or parse .env at %s", filename)
			}
			return nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return nil
		}
		directory = parent
	}
}
