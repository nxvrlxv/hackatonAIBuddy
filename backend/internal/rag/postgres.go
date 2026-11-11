package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type DocumentMetadata struct {
	ContentHash     string
	IndexingVersion string
}

func LoadDocumentMetadata(ctx context.Context, db *pgx.Conn) (map[string]DocumentMetadata, error) {
	query := "SELECT source_path, content_hash, indexing_version FROM buddy.documents"
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error while get files metadata: %w", err)
	}
	result := make(map[string]DocumentMetadata)
	defer rows.Close()

	for rows.Next() {
		var path string
		var metadata DocumentMetadata

		err := rows.Scan(
			&path,
			&metadata.ContentHash,
			&metadata.IndexingVersion,
		)
		if err != nil {
			return nil, err
		}

		result[path] = metadata
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func GetTeamId(ctx context.Context, db *pgx.Conn, teamCode string) (int64, error) {
	query := `SELECT id FROM buddy.teams where code= $1`
	var id int64
	err := db.QueryRow(ctx, query, teamCode).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// DocumentToSave describes one source and its already generated embeddings.
// SourcePath must be relative to the knowledge directory, e.g. common/access.md.
// TeamID == nil means the document is available to all teams. The caller must
// resolve the actual team for team-scoped files before calling SaveDocument.
// ContentHash must describe the same file bytes used to generate Chunks.
// The current schema stores metadata and chunks, not the original Markdown file.
type DocumentToSave struct {
	SourcePath      string
	Title           string
	TeamID          *int64
	ContentHash     string
	IndexingVersion string
	Chunks          []EmbeddedChunk
}

// DocumentDB is satisfied by *pgx.Conn and *pgxpool.Pool.
type DocumentDB interface {
	Begin(context.Context) (pgx.Tx, error)
}

// SaveDocument inserts or updates a document and replaces all of its old chunks.
// The returned ID is valid only on success. A failure rolls back metadata changes,
// deletion of old chunks and insertion of new chunks together.
func SaveDocument(ctx context.Context, db DocumentDB, document DocumentToSave) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if db == nil {
		return 0, fmt.Errorf("database is nil")
	}
	if strings.TrimSpace(document.SourcePath) == "" || strings.TrimSpace(document.Title) == "" ||
		strings.TrimSpace(document.ContentHash) == "" || strings.TrimSpace(document.IndexingVersion) == "" {
		return 0, fmt.Errorf("source path, title, content hash and indexing version are required")
	}
	if len(document.Chunks) == 0 {
		return 0, fmt.Errorf("cannot save a document without chunks")
	}
	indices := make(map[int]struct{}, len(document.Chunks))
	for _, item := range document.Chunks {
		if item.Chunk.ChunkIndex < 0 || strings.TrimSpace(item.Chunk.Content) == "" {
			return 0, fmt.Errorf("chunk %d: nonnegative index and nonempty content required", item.Chunk.ChunkIndex)
		}
		if _, exists := indices[item.Chunk.ChunkIndex]; exists {
			return 0, fmt.Errorf("duplicate chunk index %d", item.Chunk.ChunkIndex)
		}
		indices[item.Chunk.ChunkIndex] = struct{}{}
		// Matches vector(1024) in migration 0002; update together with that schema.
		if len(item.Embedding) != 1024 {
			return 0, fmt.Errorf("chunk %d: expected 1024 embedding coordinates, got %d", item.Chunk.ChunkIndex, len(item.Embedding))
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin document transaction: %w", err)
	}
	defer func() {
		// Rollback must also be attempted when the caller cancels ctx.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx) // Harmless after a successful commit.
	}()

	var documentID int64
	err = tx.QueryRow(ctx, `INSERT INTO buddy.documents
        (source_path, title, team_id, content_hash, indexing_version)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (source_path) DO UPDATE SET
            title = EXCLUDED.title,
            team_id = EXCLUDED.team_id,
            content_hash = EXCLUDED.content_hash,
            indexing_version = EXCLUDED.indexing_version,
            updated_at = now()
        RETURNING id`, document.SourcePath, document.Title, document.TeamID,
		document.ContentHash, document.IndexingVersion).Scan(&documentID)
	if err != nil {
		return 0, fmt.Errorf("upsert document %q: %w", document.SourcePath, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM buddy.chunks WHERE document_id = $1`, documentID); err != nil {
		return 0, fmt.Errorf("delete old chunks for document %d: %w", documentID, err)
	}
	for _, item := range document.Chunks {
		// Text serialization plus an explicit cast works without registering a
		// pgvector binary codec on the caller's connection. No SQL interpolation.
		vectorText := pgvector.NewVector(item.Embedding).String()
		_, err := tx.Exec(ctx, `INSERT INTO buddy.chunks
            (document_id, team_id, chunk_index, content, section_title, embedding)
            VALUES ($1, $2, $3, $4, $5, $6::vector)`, documentID, document.TeamID,
			item.Chunk.ChunkIndex, item.Chunk.Content, item.Chunk.SectionTitle, vectorText)
		if err != nil {
			return 0, fmt.Errorf("insert chunk %d for document %d: %w", item.Chunk.ChunkIndex, documentID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit document %d: %w", documentID, err)
	}
	return documentID, nil
}
