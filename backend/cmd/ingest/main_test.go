package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
	"github.com/tmc/langchaingo/embeddings"
)

func TestParseOptions(t *testing.T) {
	for _, name := range []string{"-iv", "-index-version"} {
		opts, err := parseOptions([]string{name, "v2", "-knowledge", "/knowledge"}, io.Discard)
		if err != nil || opts.indexVersion != "v2" || opts.knowledgeDir != "/knowledge" {
			t.Fatalf("%+v %v", opts, err)
		}
	}
	for _, args := range [][]string{nil, {"-iv", " "}, {"-unknown"}, {"-iv", "v1", "extra"}} {
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := parseOptions([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
}

func TestPrepareDocument(t *testing.T) {
	text := "# Access\nAsk your mentor."
	path := filepath.Join(t.TempDir(), "access.md")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	teamID := int64(7)
	file := rag.SelectedFile{FullPath: path, SourcePath: "team-payments/access.md", TeamID: &teamID, Hash: "old-hash", IndexVersion: "v3"}
	var received []string
	embedder, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		received = append(received, texts...)
		vectors := make([][]float32, len(texts))
		for i := range texts {
			vectors[i] = make([]float32, 1024)
			vectors[i][0] = 3
			vectors[i][1] = 4
		}
		return vectors, nil
	}), embeddings.WithStripNewLines(false))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := prepareDocument(context.Background(), file, embedder)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(text))
	if doc.ContentHash != hex.EncodeToString(sum[:]) || doc.SourcePath != file.SourcePath || doc.TeamID != &teamID || doc.IndexingVersion != "v3" || doc.Title != "Access" {
		t.Fatalf("incorrect metadata: %+v", doc)
	}
	if len(received) != 1 || received[0] != "Section: Access\n\nAsk your mentor." {
		t.Fatalf("wrong embedding text: %v", received)
	}
	if len(doc.Chunks) != 1 || doc.Chunks[0].Embedding[0] != 0.6 {
		t.Fatal("missing normalized chunks")
	}
}

func TestPrepareDocumentErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	file := rag.SelectedFile{FullPath: path, SourcePath: "common/empty.md", IndexVersion: "v1"}
	if _, err := prepareDocument(context.Background(), file, nil); err == nil {
		t.Fatal("missing file accepted")
	}
	if err := os.WriteFile(path, []byte("# Heading only"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDocument(context.Background(), file, nil); err == nil {
		t.Fatal("heading-only document accepted")
	}
	if err := os.WriteFile(path, []byte("Body"), 0600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("model unavailable")
	e, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(func(context.Context, []string) ([][]float32, error) { return nil, failure }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDocument(context.Background(), file, e); !errors.Is(err, failure) {
		t.Fatalf("model error lost: %v", err)
	}
}

func TestNewEmbedderRejectsInvalidSettings(t *testing.T) {
	for _, cfg := range []*config.Config{{OllamaURL: "%", EmbedModel: "test"}, {OllamaURL: "localhost:11434", EmbedModel: "test"}, {OllamaURL: "http://localhost:11434"}} {
		if _, err := newEmbedder(cfg); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
