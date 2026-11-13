package rag

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/tmc/langchaingo/embeddings"

	//"github.com/tmc/langchaingo/llms"

	"github.com/tmc/langchaingo/llms/ollama"
)

type EmbedClient struct {
	llm      *ollama.LLM
	Embedder *embeddings.EmbedderImpl
}

type SearchResult struct {
	SourcePath   string  `json:"source_path"`
	Title        string  `json:"title"`
	SectionTitle string  `json:"section_title"`
	Content      string  `json:"content"`
	Distance     float64 `json:"distance"`
}

// type Service struct {
// 	repo   *repo.Repository
// 	client *llms.ClientLLM
// 	topK   int //топ чанков брать в контекст
// }

func NewEmbedClient(cfg *config.Config) (*EmbedClient, error) {
	model, err := ollama.New(
		ollama.WithModel(cfg.EmbedModel),
		ollama.WithServerURL(cfg.OllamaURL),
		ollama.WithHTTPClient(&http.Client{Timeout: 2 * time.Minute}),
	)
	if err != nil {
		return nil, fmt.Errorf("create Ollama client: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(model, embeddings.WithStripNewLines(false))
	if err != nil {
		return nil, fmt.Errorf("create Ollama client: %w", err)
	}

	return &EmbedClient{
		llm:      model,
		Embedder: embedder,
	}, nil
}

type SearchChunks interface {
	SearchChunks(ctx context.Context, embedding []float32, teamID *int64, limit int) ([]SearchResult, error)
}

type ServiceRAG struct {
	searcherChunks SearchChunks
	client         *EmbedClient
	limit          int
	maxDistance    float64 // косинусное расстояние: чанки дальше порога считаем нерелевантными
}

func NewService(client *EmbedClient, searcher SearchChunks, limit int, maxDistance float64) *ServiceRAG {
	return &ServiceRAG{
		searcherChunks: searcher,
		client:         client,
		limit:          limit,
		maxDistance:    maxDistance,
	}
}

// Данная функция находит самый близкий контекст
func (s *ServiceRAG) Query(ctx context.Context, question string, teamID *int64) ([]SearchResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("question is empty")
	}

	vectorQ, err := s.client.Embedder.EmbedQuery(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("some error while embedding question: %w", err)
	}
	normalized, err := normalizeQueryVector(vectorQ)
	if err != nil {
		return nil, err
	}

	//БД
	result, err := s.searcherChunks.SearchChunks(ctx, normalized, teamID, s.limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks error: %w", err)
	}

	// result отсортирован по distance, поэтому после первого далёкого чанка дальше только хуже.
	// Пустой срез (не nil) означает «в базе знаний ответа нет».
	relevant := make([]SearchResult, 0, len(result))
	for _, chunk := range result {
		if chunk.Distance > s.maxDistance {
			break
		}
		relevant = append(relevant, chunk)
	}

	return relevant, nil
}

// normalizeQueryVector приводит вектор вопроса к единичной длине (L2).
// Нулевой вектор и вектор с NaN/Inf отклоняются.
func normalizeQueryVector(vector []float32) ([]float32, error) {
	// Accumulate in float64 so squaring float32 coordinates neither
	// overflows for large values nor underflows for tiny values.
	var squaredNorm float64
	for _, value := range vector {
		coordinate := float64(value)
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return nil, fmt.Errorf("question embedding contains a non-finite value")
		}
		squaredNorm += coordinate * coordinate
	}
	if squaredNorm == 0 {
		return nil, fmt.Errorf("cannot normalize a zero question embedding")
	}
	norm := math.Sqrt(squaredNorm)
	normalized := make([]float32, len(vector))
	for i, value := range vector {
		normalized[i] = float32(float64(value) / norm)
	}
	return normalized, nil
}
