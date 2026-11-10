package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/embeddings"
)

type memoryIngestStore struct {
	metadata    map[string]DocumentMetadata
	teams       map[string]int64
	batches     [][]string
	saved       []IndexedDocument
	saveErr     error
	metadataErr error
}

func (s *memoryIngestStore) LoadTeams(context.Context) (map[string]int64, error) { return s.teams, nil }
func (s *memoryIngestStore) LoadMetadata(_ context.Context, paths []string) (map[string]DocumentMetadata, error) {
	s.batches = append(s.batches, append([]string(nil), paths...))
	result := make(map[string]DocumentMetadata)
	for _, path := range paths {
		if item, ok := s.metadata[path]; ok {
			result[path] = item
		}
	}
	return result, s.metadataErr
}
func (s *memoryIngestStore) SaveDocument(_ context.Context, doc IndexedDocument) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, doc)
	s.metadata[doc.SourcePath] = DocumentMetadata{doc.ContentHash, doc.IndexingVersion, doc.TeamID}
	return nil
}
func ingestFixture(t *testing.T) (IngestOptions, *memoryIngestStore, *embeddings.EmbedderImpl, *int) {
	t.Helper()
	opts := IngestOptions{KnowledgeDir: t.TempDir(), MetadataBatchSize: 2, ChunkSize: 100, ChunkOverlap: 10,
		EmbeddingDimensions: 2, EmbeddingModel: "test-model", EmbeddingServer: "http://local/v1", IndexingVersion: "v1"}
	store := &memoryIngestStore{metadata: make(map[string]DocumentMetadata), teams: map[string]int64{"payments": 7}}
	calls := new(int)
	embedder, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(func(ctx context.Context, texts []string) ([][]float32, error) {
		*calls++
		vectors := make([][]float32, len(texts))
		for i := range texts {
			vectors[i] = []float32{3, 4}
		}
		return vectors, nil
	}), embeddings.WithBatchSize(2), embeddings.WithStripNewLines(false))
	if err != nil {
		t.Fatal(err)
	}
	return opts, store, embedder, calls
}
func putKnowledge(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func fileHash(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

func TestIngestBatchesAndIncrementalUpdates(t *testing.T) {
	opts, store, embedder, calls := ingestFixture(t)
	body := "# Access\nAsk your mentor."
	putKnowledge(t, opts.KnowledgeDir, "common/a.md", body)
	putKnowledge(t, opts.KnowledgeDir, "common/b.md", body) // Same hash, different document.
	putKnowledge(t, opts.KnowledgeDir, "team-payments/c.MD", "# Payments\nTeam instructions.")
	putKnowledge(t, opts.KnowledgeDir, "common/ignored.txt", "Ignore")
	stats, err := Ingest(context.Background(), store, embedder, opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats != (IngestStats{Files: 3, Updated: 3, Chunks: 3}) {
		t.Fatalf("wrong stats: %+v", stats)
	}
	wantBatches := [][]string{{"common/a.md", "common/b.md"}, {"team-payments/c.MD"}}
	if !reflect.DeepEqual(store.batches, wantBatches) {
		t.Fatalf("wrong batches: %#v", store.batches)
	}
	if store.saved[0].ContentHash != fileHash(body) || store.saved[0].ContentHash != store.saved[1].ContentHash {
		t.Fatal("wrong content hash")
	}
	if store.saved[0].TeamID != nil || store.saved[2].TeamID == nil || *store.saved[2].TeamID != 7 {
		t.Fatal("wrong document access scope")
	}
	if store.saved[0].Title != "Access" || store.saved[0].Chunks[0].Embedding[0] != 0.6 {
		t.Fatal("wrong title or embedding")
	}
	before := *calls
	stats, err = Ingest(context.Background(), store, embedder, opts)
	if err != nil || stats.Skipped != 3 || stats.Updated != 0 || *calls != before {
		t.Fatalf("unchanged files were embedded: %+v %v", stats, err)
	}
	putKnowledge(t, opts.KnowledgeDir, "common/a.md", body+"\nUpdated.")
	stats, err = Ingest(context.Background(), store, embedder, opts)
	if err != nil || stats.Updated != 1 || stats.Skipped != 2 {
		t.Fatalf("hash change not detected: %+v %v", stats, err)
	}
	opts.IndexingVersion = "new-weights"
	stats, err = Ingest(context.Background(), store, embedder, opts)
	if err != nil || stats.Updated != 3 {
		t.Fatalf("version change not detected: %+v %v", stats, err)
	}
	// Removed local files do not silently delete database documents.
	if err := os.Remove(filepath.Join(opts.KnowledgeDir, "common/b.md")); err != nil {
		t.Fatal(err)
	}
	_, err = Ingest(context.Background(), store, embedder, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.metadata["common/b.md"]; !ok {
		t.Fatal("missing source deleted from database")
	}
}

func TestIndexingVersionSettings(t *testing.T) {
	opts, _, _, _ := ingestFixture(t)
	base := EffectiveIndexingVersion(opts)
	changes := []func(*IngestOptions){
		func(o *IngestOptions) { o.EmbeddingModel += "-new" },
		func(o *IngestOptions) { o.EmbeddingServer += "/other" },
		func(o *IngestOptions) { o.EmbeddingDimensions++ },
		func(o *IngestOptions) { o.ChunkSize++ },
		func(o *IngestOptions) { o.ChunkOverlap++ },
		func(o *IngestOptions) { o.IndexingVersion += "-new" },
	}
	for _, change := range changes {
		changed := opts
		change(&changed)
		if EffectiveIndexingVersion(changed) == base {
			t.Fatal("settings change did not change version")
		}
	}
	opts.MetadataBatchSize++
	opts.KnowledgeDir += "other"
	opts.EmbeddingServer += "/"
	if EffectiveIndexingVersion(opts) != base {
		t.Fatal("non-semantic settings changed version")
	}
}

func TestIngestRejectsInvalidDocumentsBeforeSaving(t *testing.T) {
	tests := []struct {
		name, path, body string
		dimensions       int
	}{
		{"unknown team", "team-unknown/a.md", "body", 2},
		{"unknown scope", "other/a.md", "body", 2},
		{"root file", "a.md", "body", 2},
		{"empty", "common/a.md", "  ", 2},
		{"heading only", "common/a.md", "# Empty", 2},
		{"dimension mismatch", "common/a.md", "body", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, store, embedder, _ := ingestFixture(t)
			opts.EmbeddingDimensions = tt.dimensions
			putKnowledge(t, opts.KnowledgeDir, tt.path, tt.body)
			if _, err := Ingest(context.Background(), store, embedder, opts); err == nil || len(store.saved) != 0 {
				t.Fatalf("invalid document saved: %v", err)
			}
		})
	}
}

func TestIngestFailureAndCancellation(t *testing.T) {
	opts, store, embedder, calls := ingestFixture(t)
	putKnowledge(t, opts.KnowledgeDir, "common/a.md", "body")
	sentinel := errors.New("database unavailable")
	store.metadataErr = sentinel
	if _, err := Ingest(context.Background(), store, embedder, opts); !errors.Is(err, sentinel) || *calls != 0 {
		t.Fatal("metadata error did not prevent embedding")
	}
	store.metadataErr = nil
	store.saveErr = sentinel
	if _, err := Ingest(context.Background(), store, embedder, opts); !errors.Is(err, sentinel) || len(store.metadata) != 0 {
		t.Fatal("save failure changed metadata")
	}
	store.saveErr = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Ingest(ctx, store, embedder, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	bad, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(func(context.Context, []string) ([][]float32, error) { return nil, sentinel }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Ingest(context.Background(), store, bad, opts); !errors.Is(err, sentinel) || len(store.saved) != 0 {
		t.Fatal("embedding failure saved document")
	}
}

func TestIngestSymlinkAndTeamReassignment(t *testing.T) {
	opts, store, embedder, calls := ingestFixture(t)
	putKnowledge(t, opts.KnowledgeDir, "team-payments/a.md", "body")
	id := int64(99)
	store.metadata["team-payments/a.md"] = DocumentMetadata{fileHash("body"), EffectiveIndexingVersion(opts), &id}
	if stats, err := Ingest(context.Background(), store, embedder, opts); err != nil || stats.Updated != 1 || *calls == 0 {
		t.Fatal("changed scope was skipped")
	}
	if err := os.Symlink("a.md", filepath.Join(opts.KnowledgeDir, "team-payments/link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Ingest(context.Background(), store, embedder, opts); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink accepted: %v", err)
	}
}
