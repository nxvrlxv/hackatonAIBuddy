package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type IngestDB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Begin(context.Context) (pgx.Tx, error)
}

type PostgresIngestStore struct{ DB IngestDB }

func (s PostgresIngestStore) LoadMetadata(ctx context.Context, paths []string) (map[string]DocumentMetadata, error) {
	result := make(map[string]DocumentMetadata, len(paths))
	if len(paths) == 0 {
		return result, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT source_path, content_hash, indexing_version, team_id
        FROM buddy.documents WHERE source_path = ANY($1::text[])`, paths)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var metadata DocumentMetadata
		if err := rows.Scan(&path, &metadata.ContentHash, &metadata.IndexingVersion, &metadata.TeamID); err != nil {
			return nil, err
		}
		if _, exists := result[path]; exists {
			return nil, fmt.Errorf("duplicate document path %q; apply source identity migration", path)
		}
		result[path] = metadata
	}
	return result, rows.Err()
}

func (s PostgresIngestStore) LoadTeams(ctx context.Context) (map[string]int64, error) {
	rows, err := s.DB.Query(ctx, `SELECT code, id FROM buddy.teams`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	teams := make(map[string]int64)
	for rows.Next() {
		var code string
		var id int64
		if err := rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		if _, exists := teams[code]; exists {
			return nil, fmt.Errorf("duplicate team code %q", code)
		}
		teams[code] = id
	}
	return teams, rows.Err()
}

func (s PostgresIngestStore) SaveDocument(ctx context.Context, document IndexedDocument) error {
	if len(document.Chunks) == 0 {
		return fmt.Errorf("cannot save a document without chunks")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Cleanup still works after the request context has been canceled.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO buddy.documents
        (source_path, title, team_id, content_hash, indexing_version)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (source_path) DO UPDATE SET title = EXCLUDED.title,
            team_id = EXCLUDED.team_id, content_hash = EXCLUDED.content_hash,
            indexing_version = EXCLUDED.indexing_version, updated_at = now()
        RETURNING id`, document.SourcePath, document.Title, document.TeamID,
		document.ContentHash, document.IndexingVersion).Scan(&id)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM buddy.chunks WHERE document_id = $1`, id); err != nil {
		return err
	}
	count, err := tx.CopyFrom(ctx, pgx.Identifier{"buddy", "chunks"},
		[]string{"document_id", "team_id", "chunk_index", "content", "section_title", "embedding"},
		pgx.CopyFromSlice(len(document.Chunks), func(i int) ([]any, error) {
			chunk := document.Chunks[i]
			return []any{id, document.TeamID, chunk.Chunk.ChunkIndex, chunk.Chunk.Content,
				chunk.Chunk.SectionTitle, pgvector.NewVector(chunk.Embedding)}, nil
		}))
	if err != nil {
		return err
	}
	if count != int64(len(document.Chunks)) {
		return fmt.Errorf("expected to insert %d chunks, inserted %d", len(document.Chunks), count)
	}
	return tx.Commit(ctx)
}
