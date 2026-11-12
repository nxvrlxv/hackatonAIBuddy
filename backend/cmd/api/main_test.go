package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestRunFailsWithoutConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Fatal("run must fail without config")
	}
}

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
		"short jwt secret": {"JWT_SECRET": "short"},
		"bad database url": {"DATABASE_URL": "://bad"},
		"bad agent url":    {"AGENT_URL": "not a url"},
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

func TestServeStopsOnCancel(t *testing.T) {
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, time.Second) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown must return nil, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeReturnsListenError(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	server := &http.Server{Addr: busy.Addr().String()}
	if err := serve(context.Background(), server, time.Second); err == nil {
		t.Fatal("serve must return the listen error")
	}
}

func TestNewAgentProxy(t *testing.T) {
	for _, raw := range []string{"http://agent:8001", "https://agent.example/base"} {
		if h, err := newAgentProxy(raw, "k"); err != nil || h == nil {
			t.Fatalf("%q: unexpected error %v", raw, err)
		}
	}
	for _, raw := range []string{"", "agent:8001", "ftp://agent", "http://", "://bad"} {
		if _, err := newAgentProxy(raw, "k"); err == nil {
			t.Fatalf("%q must be rejected", raw)
		}
	}
}
