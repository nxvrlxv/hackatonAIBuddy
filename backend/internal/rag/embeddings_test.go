package rag_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
	"github.com/tmc/langchaingo/embeddings"
)

func newEmbedder(t *testing.T, fn embeddings.EmbedderClientFunc, batchSize int) *embeddings.EmbedderImpl {
	t.Helper()
	e, err := embeddings.NewEmbedder(fn, embeddings.WithBatchSize(batchSize), embeddings.WithStripNewLines(false))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestBuildEmbeddingsBatchesAndPreservesChunks(t *testing.T) {
	chunks := []rag.Chunk{
		{HeadingPath: []string{"Root", "Child"}, SectionTitle: "Root / Child", Content: "Body", EmbeddingText: "Section: Root / Child\n\nBody", ChunkIndex: 4},
		{Content: "Second", EmbeddingText: "Second", ChunkIndex: 5},
		{Content: "Third", EmbeddingText: "Third", ChunkIndex: 6},
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	var batches [][]string
	count := 0
	embedder := newEmbedder(t, func(gotCtx context.Context, texts []string) ([][]float32, error) {
		if gotCtx.Value(contextKey{}) != "request" {
			t.Fatal("context not forwarded")
		}
		batches = append(batches, append([]string(nil), texts...))
		vectors := make([][]float32, len(texts))
		for i := range texts {
			count++
			vectors[i] = []float32{float32(count), 1}
		}
		return vectors, nil
	}, 2)
	got, err := rag.BuildEmbeddings(ctx, embedder, chunks)
	if err != nil {
		t.Fatal(err)
	}
	wantBatches := [][]string{{chunks[0].EmbeddingText, chunks[1].EmbeddingText}, {chunks[2].EmbeddingText}}
	if !reflect.DeepEqual(batches, wantBatches) {
		t.Fatalf("wrong API inputs: %#v", batches)
	}
	for i, item := range got {
		if !reflect.DeepEqual(item.Chunk, chunks[i]) {
			t.Fatalf("metadata changed at %d", i)
		}
		norm := math.Sqrt(float64((i+1)*(i+1) + 1))
		want := []float64{float64(i+1) / norm, 1 / norm}
		for j, value := range item.Embedding {
			if math.Abs(float64(value)-want[j]) > 1e-6 {
				t.Fatalf("wrong normalized vector for chunk %d: %v", i, item.Embedding)
			}
		}
	}
	if chunks[0].EmbeddingText != wantBatches[0][0] {
		t.Fatal("input mutated")
	}
}

func TestBuildEmbeddingsRejectsInvalidResponse(t *testing.T) {
	chunks := []rag.Chunk{{EmbeddingText: "first"}, {EmbeddingText: "second"}}
	cases := []struct {
		name    string
		vectors [][]float32
	}{
		{"missing", [][]float32{{1, 2}}},
		{"extra", [][]float32{{1}, {2}, {3}}},
		{"empty", [][]float32{{}, {1, 2}}},
		{"different dimensions", [][]float32{{1}, {1, 2}}},
		{"nan", [][]float32{{1, 2}, {float32(math.NaN()), 1}}},
		{"infinity", [][]float32{{1, 2}, {float32(math.Inf(1)), 1}}},
		{"zero", [][]float32{{1, 2}, {0, 0}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := newEmbedder(t, func(context.Context, []string) ([][]float32, error) { return tt.vectors, nil }, 2)
			got, err := rag.BuildEmbeddings(context.Background(), e, chunks)
			if err == nil || got != nil {
				t.Fatalf("expected error without partial result, got %#v, %v", got, err)
			}
		})
	}
}

func TestBuildEmbeddingsBatchFailure(t *testing.T) {
	sentinel := errors.New("provider unavailable")
	calls := 0
	e := newEmbedder(t, func(context.Context, []string) ([][]float32, error) {
		calls++
		if calls == 2 {
			return nil, sentinel
		}
		return [][]float32{{1, 2}}, nil
	}, 1)
	got, err := rag.BuildEmbeddings(context.Background(), e, []rag.Chunk{{EmbeddingText: "a"}, {EmbeddingText: "b"}})
	if !errors.Is(err, sentinel) || got != nil {
		t.Fatalf("expected wrapped provider error, got %#v, %v", got, err)
	}
}

func TestBuildEmbeddingsValidationAndEmptyInput(t *testing.T) {
	calls := 0
	e := newEmbedder(t, func(context.Context, []string) ([][]float32, error) { calls++; return [][]float32{{1}}, nil }, 1)
	got, err := rag.BuildEmbeddings(context.Background(), e, nil)
	if err != nil || len(got) != 0 || calls != 0 {
		t.Fatal("empty input should not call provider")
	}
	if _, err := rag.BuildEmbeddings(context.Background(), e, []rag.Chunk{{EmbeddingText: " \n"}}); err == nil || calls != 0 {
		t.Fatal("empty embedding text accepted")
	}
	chunk := []rag.Chunk{{EmbeddingText: "body"}}
	if _, err := rag.BuildEmbeddings(context.Background(), nil, chunk); err == nil {
		t.Fatal("nil client accepted")
	}
	for _, size := range []int{0, -1} {
		e.BatchSize = size
		if _, err := rag.BuildEmbeddings(context.Background(), e, chunk); err == nil {
			t.Fatal("invalid batch size accepted")
		}
	}
	if calls != 0 {
		t.Fatal("provider called for invalid input")
	}
}

func TestBuildEmbeddingsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := rag.BuildEmbeddings(ctx, nil, []rag.Chunk{{EmbeddingText: "a"}}); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("expected context cancellation: %#v, %v", got, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	e := newEmbedder(t, func(ctx context.Context, _ []string) ([][]float32, error) {
		cancel()
		return nil, ctx.Err()
	}, 1)
	if got, err := rag.BuildEmbeddings(ctx, e, []rag.Chunk{{EmbeddingText: "a"}}); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("expected provider cancellation: %#v, %v", got, err)
	}
}

func TestBuildEmbeddingsL2Normalization(t *testing.T) {
	cases := []struct {
		name   string
		vector []float32
		want   []float32
	}{
		{"positive", []float32{3, 4}, []float32{0.6, 0.8}},
		{"negative", []float32{-3, 0, 4}, []float32{-0.6, 0, 0.8}},
		{"unit vector", []float32{0, -1}, []float32{0, -1}},
		{"large", []float32{math.MaxFloat32, math.MaxFloat32}, []float32{float32(1 / math.Sqrt(2)), float32(1 / math.Sqrt(2))}},
		{"tiny", []float32{math.SmallestNonzeroFloat32, math.SmallestNonzeroFloat32}, []float32{float32(1 / math.Sqrt(2)), float32(1 / math.Sqrt(2))}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			original := append([]float32(nil), tt.vector...)
			e := newEmbedder(t, func(context.Context, []string) ([][]float32, error) {
				return [][]float32{tt.vector}, nil
			}, 1)
			got, err := rag.BuildEmbeddings(context.Background(), e, []rag.Chunk{{EmbeddingText: "body"}})
			if err != nil {
				t.Fatal(err)
			}
			vector := got[0].Embedding
			if len(vector) != len(tt.want) {
				t.Fatalf("dimension changed: %d", len(vector))
			}
			var squaredNorm float64
			for i, value := range vector {
				if math.Abs(float64(value)-float64(tt.want[i])) > 1e-6 {
					t.Fatalf("coordinate %d: got %v, want %v", i, value, tt.want[i])
				}
				squaredNorm += float64(value) * float64(value)
			}
			if math.Abs(math.Sqrt(squaredNorm)-1) > 1e-6 {
				t.Fatalf("vector is not unit length: %v", vector)
			}
			if !reflect.DeepEqual(tt.vector, original) {
				t.Fatal("provider vector mutated")
			}
			vector[0] = 42
			if !reflect.DeepEqual(tt.vector, original) {
				t.Fatal("result aliases provider vector")
			}
		})
	}
}
