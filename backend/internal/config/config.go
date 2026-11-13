package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	OllamaURL   string
	EmbedModel  string
	LLMModel    string
	Port        string
	JWTSecret   string
	FileStore   string
	AgentKey    string // общий секрет api ↔ agent (заголовок X-Agent-Key)
	AgentURL    string // адрес Python-агента
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

	agentKey := os.Getenv("AGENT_KEY")
	if agentKey == "" {
		return nil, fmt.Errorf("empty agent key")
	}

	agentURL := os.Getenv("AGENT_URL")
	if agentURL == "" {
		agentURL = "http://agent:8001"
	}

	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
		LLMModel:    llm,
		OllamaURL:   ollamaUrl,
		JWTSecret:   jwtSecret,
		EmbedModel:  embedModel,
		FileStore:   fileStore,
		AgentKey:    agentKey,
		AgentURL:    agentURL,
	}, nil
}
