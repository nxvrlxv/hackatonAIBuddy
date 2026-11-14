package main

import "testing"

func setEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	env := map[string]string{
		"DATABASE_URL": "postgres://u:p@127.0.0.1:1/db", "OLLAMA_URL": "http://127.0.0.1:1",
		"EMBEDDED_MODEL": "e", "LLM_MODEL": "l", "KNOWLEDGE_PATH": "k",
		"JWT_SECRET": "test-secret-that-is-at-least-32-bytes-long", "AGENT_KEY": "k",
	}
	for k, v := range overrides {
		env[k] = v
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestRunStartupErrors(t *testing.T) {
	tests := map[string]map[string]string{
		"missing config":   {"JWT_SECRET": ""},
		"short jwt secret": {"JWT_SECRET": "short"},
		"bad database url": {"DATABASE_URL": "://bad"},
		"db unreachable":   {},
	}
	for name, overrides := range tests {
		t.Run(name, func(t *testing.T) {
			setEnv(t, overrides)
			if err := run(); err == nil {
				t.Fatal("run must fail")
			}
		})
	}
}
