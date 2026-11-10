package rag

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestPostgresLoadMetadataBatch(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	paths := []string{"common/a.md", "common/b.md"}
	db.ExpectQuery(`SELECT source_path, content_hash, indexing_version, team_id FROM buddy.documents WHERE source_path = ANY`).
		WithArgs(paths).WillReturnRows(pgxmock.NewRows([]string{"source_path", "content_hash", "indexing_version", "team_id"}).
		AddRow("common/a.md", "hash", "v1", nil)).RowsWillBeClosed()
	store := PostgresIngestStore{DB: db}
	got, err := store.LoadMetadata(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]DocumentMetadata{"common/a.md": {ContentHash: "hash", IndexingVersion: "v1"}}) {
		t.Fatalf("wrong metadata: %#v", got)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSaveDocumentAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit"
		if fail {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			document := IndexedDocument{SourcePath: "common/a.md", Title: "A", ContentHash: "hash", IndexingVersion: "v1",
				Chunks: []EmbeddedChunk{{Chunk: Chunk{Content: "body", ChunkIndex: 0}, Embedding: []float32{1, 0}}}}
			db.ExpectBegin()
			db.ExpectQuery(`INSERT INTO buddy.documents`).WithArgs(document.SourcePath, document.Title, document.TeamID, document.ContentHash, document.IndexingVersion).
				WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(42)))
			db.ExpectExec(`DELETE FROM buddy.chunks WHERE document_id`).WithArgs(int64(42)).WillReturnResult(pgxmock.NewResult("DELETE", 2))
			copy := db.ExpectCopyFrom(pgx.Identifier{"buddy", "chunks"}, []string{"document_id", "team_id", "chunk_index", "content", "section_title", "embedding"})
			sentinel := errors.New("copy failed")
			if fail {
				copy.WillReturnError(sentinel)
			} else {
				copy.WillReturnResult(1)
				db.ExpectCommit()
			}
			// The cleanup rollback is safe after a real committed pgx transaction.
			db.ExpectRollback()
			err = (PostgresIngestStore{DB: db}).SaveDocument(context.Background(), document)
			if fail && !errors.Is(err, sentinel) {
				t.Fatalf("missing copy error: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresReadFailure(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sentinel := errors.New("read interrupted")
	db.ExpectQuery(`SELECT source_path`).WithArgs([]string{"a"}).WillReturnRows(
		pgxmock.NewRows([]string{"source_path", "content_hash", "indexing_version", "team_id"}).AddRow("a", "h", "v", nil).RowError(0, sentinel))
	if _, err := (PostgresIngestStore{DB: db}).LoadMetadata(context.Background(), []string{"a"}); !errors.Is(err, sentinel) {
		t.Fatalf("row error lost: %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
