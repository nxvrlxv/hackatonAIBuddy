package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

var errDB = errors.New("db down")

// newMock создаёт мок-пул и Postgres поверх него; в конце теста проверяет, что все ожидания выполнены.
func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Postgres) {
	t.Helper()
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		db.Close()
	})
	return db, NewPostgres(db)
}

func TestEnsureTasks(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectExec(`insert into buddy.intern_tasks`).WithArgs(int64(3)).
			WillReturnResult(pgxmock.NewResult("INSERT", 11))
		if err := p.EnsureTasks(ctx, 3); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectExec(`insert into buddy.intern_tasks`).WithArgs(int64(3)).WillReturnError(errDB)
		if err := p.EnsureTasks(ctx, 3); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

var taskCols = []string{"code", "stage_title", "id", "title", "description", "status", "completed_at"}

func TestListTasks(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("rows", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id = \$1`).WithArgs(int64(3)).WillReturnRows(
			pgxmock.NewRows(taskCols).
				AddRow("team", "Знакомство с командой", int64(1), "Встреча с наставником", "описание", "done", &now).
				AddRow("tools", "Доступы и инструменты", int64(4), "Доступ к GitLab", "описание", "todo", nil))
		rows, err := p.ListTasks(ctx, 3)
		if err != nil || len(rows) != 2 {
			t.Fatalf("rows: %+v %v", rows, err)
		}
		first := rows[0]
		if first.StageCode != "team" || first.StageTitle != "Знакомство с командой" ||
			first.Task.ID != 1 || first.Task.Title != "Встреча с наставником" ||
			first.Task.Status != StatusDone || first.Task.CompletedAt == nil {
			t.Fatalf("first row: %+v", first)
		}
		if rows[1].Task.Status != StatusTodo || rows[1].Task.CompletedAt != nil {
			t.Fatalf("todo row must have no completed_at: %+v", rows[1])
		}
	})

	t.Run("empty is not nil", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id`).WithArgs(int64(3)).WillReturnRows(pgxmock.NewRows(taskCols))
		rows, err := p.ListTasks(ctx, 3)
		if err != nil || rows == nil || len(rows) != 0 {
			t.Fatalf("empty: %#v %v", rows, err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id`).WithArgs(int64(3)).WillReturnError(errDB)
		if _, err := p.ListTasks(ctx, 3); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})

	t.Run("scan error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id`).WithArgs(int64(3)).WillReturnRows(
			pgxmock.NewRows(taskCols).AddRow("team", "x", "not-a-number", "t", "d", "todo", nil))
		if _, err := p.ListTasks(ctx, 3); err == nil {
			t.Fatal("want scan error")
		}
	})

	t.Run("rows.Err", func(t *testing.T) {
		// CloseError доходит до rows.Err(); RowError в pgxmock срабатывает на Scan.
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id`).WithArgs(int64(3)).WillReturnRows(
			pgxmock.NewRows(taskCols).CloseError(errDB))
		if _, err := p.ListTasks(ctx, 3); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

var updCols = []string{"id", "title", "description", "status", "completed_at"}

func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("done", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "done").
			WillReturnRows(pgxmock.NewRows(updCols).AddRow(int64(7), "Доступ к GitLab", "описание", "done", &now))
		task, err := p.UpdateStatus(ctx, 3, 7, StatusDone)
		if err != nil || task.ID != 7 || task.Title != "Доступ к GitLab" ||
			task.Status != StatusDone || task.CompletedAt == nil {
			t.Fatalf("done: %+v %v", task, err)
		}
	})

	t.Run("back to todo clears date", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "todo").
			WillReturnRows(pgxmock.NewRows(updCols).AddRow(int64(7), "Доступ к GitLab", "описание", "todo", nil))
		task, err := p.UpdateStatus(ctx, 3, 7, StatusTodo)
		if err != nil || task.Status != StatusTodo || task.CompletedAt != nil {
			t.Fatalf("todo: %+v %v", task, err)
		}
	})

	t.Run("foreign or missing task", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(99), "in_progress").
			WillReturnError(pgx.ErrNoRows)
		if _, err := p.UpdateStatus(ctx, 3, 99, StatusInProgress); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "done").WillReturnError(errDB)
		_, err := p.UpdateStatus(ctx, 3, 7, StatusDone)
		if !errors.Is(err, errDB) || errors.Is(err, ErrNotFound) {
			t.Fatalf("want errDB (not ErrNotFound), got %v", err)
		}
	})
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusTodo, StatusInProgress, StatusDone} {
		if err := s.Valid(); err != nil {
			t.Fatalf("%q must be valid, got %v", s, err)
		}
	}
	for _, s := range []Status{"", "DONE", "closed"} {
		if err := s.Valid(); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("%q must be ErrInvalidStatus, got %v", s, err)
		}
	}
}
