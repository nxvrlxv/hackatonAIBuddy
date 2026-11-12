package buddy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/pashagolub/pgxmock/v4"
)

var userCols = []string{"id", "login", "full_name", "role", "team_id", "team", "mentor"}

func TestGetUser(t *testing.T) {
	mentor := "Анна Смирнова"
	type getter func(r *Repository) (*auth.User, error)
	cases := []struct {
		name  string
		query string
		arg   any
		get   getter
	}{
		{"by login", `where u.login = \$1`, "sasha.intern", func(r *Repository) (*auth.User, error) {
			return r.GetUserByLogin(context.Background(), "sasha.intern")
		}},
		{"by id", `where u.id = \$1`, int64(3), func(r *Repository) (*auth.User, error) {
			return r.GetUserByID(context.Background(), 3)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, repo := newMockRepo(t)

			// Найден, с ментором.
			db.ExpectQuery(tc.query).WithArgs(tc.arg).
				WillReturnRows(pgxmock.NewRows(userCols).AddRow(int64(3), "sasha.intern", "Саша Иванов", "intern", int64(2), "Путешествия", &mentor))
			u, err := tc.get(repo)
			if err != nil || u.ID != 3 || u.Login != "sasha.intern" || u.TeamID != 2 || u.Mentor == nil || *u.Mentor != mentor {
				t.Fatalf("found: %+v %v", u, err)
			}

			// Найден, без ментора.
			db.ExpectQuery(tc.query).WithArgs(tc.arg).
				WillReturnRows(pgxmock.NewRows(userCols).AddRow(int64(3), "anna", "Анна", "mentor", int64(2), "Путешествия", nil))
			if u, err := tc.get(repo); err != nil || u.Mentor != nil {
				t.Fatalf("no mentor: %+v %v", u, err)
			}

			// Нет строки → auth.ErrNotFound.
			db.ExpectQuery(tc.query).WithArgs(tc.arg).WillReturnError(pgx.ErrNoRows)
			if u, err := tc.get(repo); !errors.Is(err, auth.ErrNotFound) || u != nil {
				t.Fatalf("not found: %+v %v", u, err)
			}

			// Ошибка базы пробрасывается и не маскируется под ErrNotFound.
			db.ExpectQuery(tc.query).WithArgs(tc.arg).WillReturnError(errDB)
			if _, err := tc.get(repo); !errors.Is(err, errDB) || errors.Is(err, auth.ErrNotFound) {
				t.Fatalf("db error: %v", err)
			}

			// Ошибка сканирования (неверный тип колонки).
			db.ExpectQuery(tc.query).WithArgs(tc.arg).
				WillReturnRows(pgxmock.NewRows(userCols).AddRow("bad", "x", "x", "intern", int64(2), "t", nil))
			if _, err := tc.get(repo); err == nil || errors.Is(err, auth.ErrNotFound) {
				t.Fatalf("scan error: %v", err)
			}
			checkMock(t, db)
		})
	}
}
