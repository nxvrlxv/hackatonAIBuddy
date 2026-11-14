package buddy

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

var teamCols = []string{"id", "full_name", "role", "is_my_mentor"}

func TestListTeamMembers(t *testing.T) {
	ctx := context.Background()
	const q = `where u.team_id = \$1 and u.id <> \$2`

	t.Run("ok", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(int64(2), int64(3)).WillReturnRows(pgxmock.NewRows(teamCols).
			AddRow(int64(7), "Игорь Лебедев", "lead", false).
			AddRow(int64(1), "Анна Смирнова", "mentor", true))
		m, err := repo.ListTeamMembers(ctx, 2, 3)
		if err != nil || len(m) != 2 || m[0].Role != "lead" || !m[1].IsMyMentor || m[1].FullName != "Анна Смирнова" {
			t.Fatalf("%+v %v", m, err)
		}
	})
	t.Run("empty is not nil", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(int64(2), int64(3)).WillReturnRows(pgxmock.NewRows(teamCols))
		if m, err := repo.ListTeamMembers(ctx, 2, 3); err != nil || m == nil || len(m) != 0 {
			t.Fatalf("%#v %v", m, err)
		}
	})
	t.Run("query error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(int64(2), int64(3)).WillReturnError(errDB)
		if _, err := repo.ListTeamMembers(ctx, 2, 3); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})
	t.Run("scan error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(int64(2), int64(3)).WillReturnRows(pgxmock.NewRows(teamCols).AddRow("x", "a", "b", false))
		if _, err := repo.ListTeamMembers(ctx, 2, 3); err == nil {
			t.Fatal("want scan error")
		}
	})
	t.Run("rows.Err", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(q).WithArgs(int64(2), int64(3)).WillReturnRows(pgxmock.NewRows(teamCols).CloseError(errDB))
		if _, err := repo.ListTeamMembers(ctx, 2, 3); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})
}
