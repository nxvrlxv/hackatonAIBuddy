package rag

import (
	"context"
	"fmt"

	"math"
	"strings"

	"github.com/tmc/langchaingo/embeddings"
)

// EmbeddedChunk pairs the source chunk with its document embedding.
type EmbeddedChunk struct {
	Chunk     Chunk
	Embedding []float32
}

// BuildEmbeddings embeds only EmbeddingText, preserving input order and metadata.
// The caller configures the model, timeout and positive batch size on the embedder.
// All vectors must have the same nonzero dimension; the repository must also check
// that this dimension matches its database column (currently vector(1024)).
// Returned embeddings are L2-normalized to unit length (within float32 precision).
// Provider-owned vectors are not modified. Zero and non-finite vectors are rejected.
// On any failure no partial result is returned.
func BuildEmbeddings(ctx context.Context, embedder *embeddings.EmbedderImpl, chunks []Chunk) ([]EmbeddedChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build embeddings: %w", err)
	}
	if len(chunks) == 0 {
		return []EmbeddedChunk{}, nil
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedding client is nil")
	}
	if embedder.BatchSize <= 0 {
		return nil, fmt.Errorf("embedding batch size must be positive")
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		if strings.TrimSpace(chunk.EmbeddingText) == "" {
			return nil, fmt.Errorf("chunk %d: embedding text is empty", chunk.ChunkIndex)
		}
		texts[i] = chunk.EmbeddingText
	}
	vectors, err := embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed documents: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build embeddings: %w", err)
	}
	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(chunks), len(vectors))
	}
	dimension := len(vectors[0])
	result := make([]EmbeddedChunk, len(chunks))
	for i, vector := range vectors {
		if len(vector) == 0 || len(vector) != dimension {
			return nil, fmt.Errorf("chunk %d: invalid embedding dimension %d (expected %d)", chunks[i].ChunkIndex, len(vector), dimension)
		}
		// Accumulate in float64 so squaring float32 coordinates neither
		// overflows for large values nor underflows for tiny values.
		var squaredNorm float64
		for _, value := range vector {
			coordinate := float64(value)
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return nil, fmt.Errorf("chunk %d: embedding contains a non-finite value", chunks[i].ChunkIndex)
			}
			squaredNorm += coordinate * coordinate
		}
		if squaredNorm == 0 {
			return nil, fmt.Errorf("chunk %d: cannot normalize a zero embedding", chunks[i].ChunkIndex)
		}
		norm := math.Sqrt(squaredNorm)
		normalized := make([]float32, len(vector))
		for j, value := range vector {
			normalized[j] = float32(float64(value) / norm)
		}
		result[i] = EmbeddedChunk{Chunk: chunks[i], Embedding: normalized}
	}
	return result, nil
}
