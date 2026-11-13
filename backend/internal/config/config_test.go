package config

import (
	"os"
	"strings"
	"testing"
)

var configKeys = []string{"DATABASE_URL", "OLLAMA_URL", "EMBEDDED_MODEL", "LLM_MODEL", "JWT_SECRET", "KNOWLEDGE_PATH", "PORT", "AGENT_KEY", "AGENT_URL"}

// requiredKeys — без них Load обязан вернуть ошибку.
var requiredKeys = []string{"DATABASE_URL", "OLLAMA_URL", "EMBEDDED_MODEL", "LLM_MODEL", "JWT_SECRET", "KNOWLEDGE_PATH", "AGENT_KEY"}

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range configKeys {
		t.Setenv(key, "") // testing вернёт исходное значение после теста
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func setRequired(t *testing.T) {
	t.Helper()
	for _, key := range requiredKeys {
		t.Setenv(key, "from-environment")
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	cleanConfigEnv(t)
	setRequired(t)
	t.Setenv("AGENT_KEY", "test-agent-key")
	t.Setenv("EMBEDDED_MODEL", "test-embed")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentKey != "test-agent-key" || cfg.EmbedModel != "test-embed" || cfg.DatabaseURL != "from-environment" {
		t.Fatal("config values were not loaded correctly")
	}
}

func TestLoadDefaults(t *testing.T) {
	cleanConfigEnv(t)
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.AgentURL != "http://agent:8001" {
		t.Fatalf("AgentURL = %q, want http://agent:8001", cfg.AgentURL)
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	cleanConfigEnv(t)
	setRequired(t)
	t.Setenv("PORT", "9000")
	t.Setenv("AGENT_URL", "http://localhost:8001")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9000" || cfg.AgentURL != "http://localhost:8001" {
		t.Fatalf("defaults were not overridden: port %q, agent url %q", cfg.Port, cfg.AgentURL)
	}
}

func TestLoadRequiresEachVariable(t *testing.T) {
	for _, missing := range requiredKeys {
		t.Run(missing, func(t *testing.T) {
			cleanConfigEnv(t)
			setRequired(t)
			t.Setenv("JWT_SECRET", "sensitive-test-value")
			t.Setenv("AGENT_KEY", "sensitive-agent-value")
			if err := os.Unsetenv(missing); err != nil {
				t.Fatal(err)
			}

			_, err := Load()
			if err == nil {
				t.Fatalf("Load must fail without %s", missing)
			}
			// секреты не должны попадать в текст ошибки
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatal("secret leaked in error")
			}
		})
	}
}
