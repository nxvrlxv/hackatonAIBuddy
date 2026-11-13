package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/pgvector/pgvector-go"
)

func documentFixture() DocumentToSave {
	vector := make([]float32, 1024)
	vector[0] = 1
	return DocumentToSave{SourcePath: "common/access.md", Title: "Access", ContentHash: "sha256-of-source", IndexingVersion: "v1",
		Chunks: []EmbeddedChunk{
			{Chunk: Chunk{ChunkIndex: 0, Content: "First", SectionTitle: "Access"}, Embedding: vector},
			{Chunk: Chunk{ChunkIndex: 1, Content: "Second", SectionTitle: "Access / Request"}, Embedding: vector},
		}}
}

func TestSaveDocumentTransaction(t *testing.T) {
	for _, scenario := range []string{"common", "team", "upsert error", "delete error", "second chunk error", "commit error"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			doc := documentFixture()
			if scenario == "team" {
				id := int64(7)
				doc.TeamID = &id
				doc.SourcePath = "team-payments/access.md"
			}
			failure := errors.New("database failure")
			db.ExpectBegin()
			upsert := db.ExpectQuery(`(?s)INSERT INTO buddy.documents.*ON CONFLICT \(source_path\) DO UPDATE SET.*RETURNING id`).
				WithArgs(doc.SourcePath, doc.Title, doc.TeamID, doc.ContentHash, doc.IndexingVersion)
			if scenario == "upsert error" {
				upsert.WillReturnError(failure)
			} else {
				upsert.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(42)))
				deletion := db.ExpectExec(`DELETE FROM buddy.chunks WHERE document_id = \$1`).WithArgs(int64(42))
				if scenario == "delete error" {
					deletion.WillReturnError(failure)
				} else {
					deletion.WillReturnResult(pgxmock.NewResult("DELETE", 5))
					for i, item := range doc.Chunks {
						insert := db.ExpectExec(`INSERT INTO buddy.chunks`).WithArgs(int64(42), doc.TeamID,
							item.Chunk.ChunkIndex, item.Chunk.Content, item.Chunk.SectionTitle, pgvector.NewVector(item.Embedding).String())
						if scenario == "second chunk error" && i == 1 {
							insert.WillReturnError(failure)
						} else {
							insert.WillReturnResult(pgxmock.NewResult("INSERT", 1))
						}
					}
					if scenario != "second chunk error" {
						commit := db.ExpectCommit()
						if scenario == "commit error" {
							commit.WillReturnError(failure)
						}
					}
				}
			}
			db.ExpectRollback()
			id, err := SaveDocument(context.Background(), db, doc)
			if scenario == "common" || scenario == "team" {
				if err != nil || id != 42 {
					t.Fatalf("save: id=%d err=%v", id, err)
				}
			} else if !errors.Is(err, failure) || id != 0 {
				t.Fatalf("expected wrapped error and no ID: id=%d err=%v", id, err)
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSaveDocumentRejectsInvalidInputBeforeTransaction(t *testing.T) {
	tests := []struct {
		name   string
		change func(*DocumentToSave)
	}{
		{"no chunks", func(d *DocumentToSave) { d.Chunks = nil }},
		{"wrong dimension", func(d *DocumentToSave) { d.Chunks[0].Embedding = []float32{1} }},
		{"empty version", func(d *DocumentToSave) { d.IndexingVersion = " " }},
		{"empty path", func(d *DocumentToSave) { d.SourcePath = "" }},
		{"duplicate index", func(d *DocumentToSave) { d.Chunks[1].Chunk.ChunkIndex = 0 }},
		{"empty content", func(d *DocumentToSave) { d.Chunks[0].Chunk.Content = " " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			doc := documentFixture()
			tt.change(&doc)
			if id, err := SaveDocument(context.Background(), db, doc); err == nil || id != 0 {
				t.Fatal("invalid input accepted")
			}
			if err := db.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
