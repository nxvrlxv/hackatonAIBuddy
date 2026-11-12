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
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
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

// func NewService(client *EmbedClient, topK int) *Service {
// 	return &Service{
// 		// repo:   repository,
// 		client: client,
// 		topK:   topK,
// 	}
// }

// Данная функция находит самый близкий контекст
func Query(ctx context.Context, embedClient EmbedClient, question string, db *pgx.Conn, teamID *int64) ([]SearchResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("question is empty")
	}

	vectorQ, err := embedClient.Embedder.EmbedQuery(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("some error while embedding question: %w", err)
	}
	// Accumulate in float64 so squaring float32 coordinates neither
	// overflows for large values nor underflows for tiny values.
	var squaredNorm float64
	for _, value := range vectorQ {
		coordinate := float64(value)
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return nil, fmt.Errorf("chunk: question embedding contains a non-finite value")
		}
		squaredNorm += coordinate * coordinate
	}
	if squaredNorm == 0 {
		return nil, fmt.Errorf("chunk: cannot normalize a zero question embedding")
	}
	norm := math.Sqrt(squaredNorm)
	normalized := make([]float32, len(vectorQ))
	for j, value := range vectorQ {
		normalized[j] = float32(float64(value) / norm)
	}

	//дальше формируем промпт: вопрос полльзователя + системный промпт + контекст из sources
	queryDB := `SELECT
    				d.source_path,
    				d.title,
    				c.section_title,
    				c.content,
    				c.embedding <=> $2::vector AS distance
				FROM buddy.chunks c
				JOIN buddy.documents d ON d.id = c.document_id
				WHERE c.team_id IS NULL OR c.team_id = $1
				ORDER BY distance
				LIMIT $3;`
	// вместо 3 добавить параметр из структуры Service потом
	vectorText := pgvector.NewVector(normalized).String()
	rows, err := db.Query(ctx, queryDB, teamID, vectorText, 5)
	if err != nil {
		return nil, fmt.Errorf("RAG Search: %w", err)
	}
	defer rows.Close()
	var result []SearchResult
	for rows.Next() {
		var chunk SearchResult

		err := rows.Scan(
			&chunk.SourcePath,
			&chunk.Title,
			&chunk.SectionTitle,
			&chunk.Content,
			&chunk.Distance,
		)

		if err != nil {
			return nil, err
		}
		result = append(result, chunk)

	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read search results: %w", err)
	}
	return result, nil
}

// func buildContext(sources []repo.Chunk) string {
// 	var builder strings.Builder
// 	for i, v := range sources {
// 		builder.WriteString(fmt.Sprintf("------------\nДокумент №%d\n%s\n", i, v.Content))
// 	}

// 	return builder.String()
// }
