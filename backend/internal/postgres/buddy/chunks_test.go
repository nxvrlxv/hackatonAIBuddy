package buddy

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/pgvector/pgvector-go"
)

func TestSearchChunks(t *testing.T) {
	embedding := []float32{0.5, 0.25}
	vec := pgvector.NewVector(embedding).String()
	q := `(?s)FROM buddy.chunks c.*WHERE c.team_id IS NULL OR c.team_id = \$1.*LIMIT \$3`
	cols := []string{"source_path", "title", "section_title", "content", "distance"}
	team := int64(4)

	t.Run("with team", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(&team, vec, 3).WillReturnRows(pgxmock.NewRows(cols).
			AddRow("team-payments/a.md", "A", "A / B", "text a", 0.1).
			AddRow("common/b.md", "B", "B", "text b", 0.2))
		res, err := repo.SearchChunks(context.Background(), embedding, &team, 3)
		if err != nil || len(res) != 2 || res[0].SourcePath != "team-payments/a.md" || res[1].Distance != 0.2 {
			t.Fatalf("got %+v %v", res, err)
		}
		checkMock(t, db)
	})

	t.Run("without team", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs((*int64)(nil), vec, 5).WillReturnRows(pgxmock.NewRows(cols).
			AddRow("common/b.md", "B", "B", "text b", 0.3))
		res, err := repo.SearchChunks(context.Background(), embedding, nil, 5)
		if err != nil || len(res) != 1 || res[0].Title != "B" {
			t.Fatalf("got %+v %v", res, err)
		}
		checkMock(t, db)
	})

	t.Run("empty", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs((*int64)(nil), vec, 5).WillReturnRows(pgxmock.NewRows(cols))
		if res, err := repo.SearchChunks(context.Background(), embedding, nil, 5); err != nil || len(res) != 0 {
			t.Fatalf("got %+v %v", res, err)
		}
		checkMock(t, db)
	})

	t.Run("query error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs((*int64)(nil), vec, 5).WillReturnError(errDB)
		if _, err := repo.SearchChunks(context.Background(), embedding, nil, 5); !errors.Is(err, errDB) {
			t.Fatalf("got %v", err)
		}
		checkMock(t, db)
	})

	t.Run("scan error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs((*int64)(nil), vec, 5).WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a.md", "A", "A", "text", "not-a-float"))
		if _, err := repo.SearchChunks(context.Background(), embedding, nil, 5); err == nil {
			t.Fatal("expected scan error")
		}
		checkMock(t, db)
	})

	t.Run("rows error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs((*int64)(nil), vec, 5).WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a.md", "A", "A", "text", 0.1).
			AddRow("b.md", "B", "B", "text", 0.2).CloseError(errDB))
		if _, err := repo.SearchChunks(context.Background(), embedding, nil, 5); !errors.Is(err, errDB) {
			t.Fatalf("got %v", err)
		}
		checkMock(t, db)
	})
}
