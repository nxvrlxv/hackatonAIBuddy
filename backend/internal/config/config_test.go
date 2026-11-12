package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var configKeys = []string{"DATABASE_URL", "OLLAMA_URL", "EMBEDDED_MODEL", "LLM_MODEL", "JWT_SECRET", "KNOWLEDGE_PATH", "PORT"}

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range configKeys {
		t.Setenv(key, "") // Arrange restoration of the original value by testing.
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func envProject(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend", "cmd", "ingest"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backend", "go.mod"), []byte("module test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const testEnv = `# Test values only
DATABASE_URL="postgres://test:test@localhost/test"
OLLAMA_URL=http://localhost:11434
EMBEDDED_MODEL=test-embed
LLM_MODEL=test-chat
JWT_SECRET='test secret # inside quotes'
KNOWLEDGE_PATH=data/knowledge
`

func TestLoadRootEnvFromNestedDirectories(t *testing.T) {
	for _, relative := range []string{".", "backend", "backend/cmd/ingest"} {
		t.Run(relative, func(t *testing.T) {
			cleanConfigEnv(t)
			root := envProject(t, testEnv)
			// Only the repository's root .env should be loaded.
			if err := os.WriteFile(filepath.Join(root, "backend", ".env"), []byte("PORT=9999\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(filepath.Join(root, relative))
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.EmbedModel != "test-embed" || cfg.JWTSecret != "test secret # inside quotes" || cfg.FileStore != "data/knowledge" || cfg.Port != "8080" {
				t.Fatal("config values were not loaded correctly")
			}
		})
	}
}

func TestEnvironmentOverridesDotEnv(t *testing.T) {
	cleanConfigEnv(t)
	t.Chdir(envProject(t, testEnv))
	t.Setenv("EMBEDDED_MODEL", "deployed-model")
	t.Setenv("PORT", "9000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmbedModel != "deployed-model" || cfg.Port != "9000" {
		t.Fatal("environment was overwritten")
	}
}

func TestLoadWithoutDotEnv(t *testing.T) {
	cleanConfigEnv(t)
	t.Chdir(envProject(t, ""))
	for _, key := range configKeys {
		t.Setenv(key, "from-environment")
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedDotEnvDoesNotLeakContents(t *testing.T) {
	cleanConfigEnv(t)
	t.Chdir(envProject(t, "JWT_SECRET=\"sensitive-test-value\n"))
	_, err := Load()
	if err == nil {
		t.Fatal("malformed dotenv accepted")
	}
	if strings.Contains(err.Error(), "sensitive-test-value") {
		t.Fatal("secret leaked in error")
	}
}
