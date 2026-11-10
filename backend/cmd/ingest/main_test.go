package main

import (
	"context"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
)

func TestRunRejectsMissingSettingsBeforeConnecting(t *testing.T) {
	for _, cfg := range []config.Ingest{
		{}, {EmbeddingServer: "file:///tmp/model"}, {EmbeddingServer: "http://localhost/v1", EmbeddingModel: "model"},
	} {
		if err := run(context.Background(), cfg); err == nil {
			t.Fatal("invalid settings accepted")
		}
	}
}
