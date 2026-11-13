package buddy

import (
	"context"
	"errors"
	"fmt"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"
	"github.com/pgvector/pgvector-go"
)

func (r *Repository) SearchChunks(ctx context.Context, embedding []float32, teamID *int64, limit int) ([]rag.SearchResult, error) {
	const q = `
		SELECT d.source_path, d.title, c.section_title, c.content,
		       c.embedding <=> $2::vector AS distance
		FROM buddy.chunks c
		JOIN buddy.documents d ON d.id = c.document_id
		WHERE c.team_id IS NULL OR c.team_id = $1
		ORDER BY distance
		LIMIT $3`
	rows, err := r.pool.Query(ctx, q, teamID, pgvector.NewVector(embedding).String(), limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	var result []rag.SearchResult
	for rows.Next() {
		var searchResult rag.SearchResult
		err := rows.Scan(&searchResult.SourcePath, &searchResult.Title, &searchResult.SectionTitle, &searchResult.Content, &searchResult.Distance)
		if err != nil {
			if errors.Is(err, auth.ErrNotFound) {
				return nil, fmt.Errorf("rows scan not found: %w", err)
			}
			return nil, fmt.Errorf("rows scan: %w", err)
		}
		result = append(result, searchResult)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read search results: %w", err)
	}

	return result, nil
}
