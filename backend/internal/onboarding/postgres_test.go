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

var taskCols = []string{"code", "stage_title", "id", "title", "description", "status", "completed_at", "submitted_at", "review_comment"}

func TestListTasks(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	comment := "Нет доступа к репозиторию"

	t.Run("rows", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where it.user_id = \$1`).WithArgs(int64(3)).WillReturnRows(
			pgxmock.NewRows(taskCols).
				AddRow("team", "Знакомство с командой", int64(1), "Встреча с наставником", "описание", "done", &now, nil, nil).
				AddRow("tools", "Доступы и инструменты", int64(4), "Доступ к GitLab", "описание", "in_progress", nil, nil, &comment))
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
		if rows[1].Task.Status != StatusInProgress || rows[1].Task.CompletedAt != nil ||
			rows[1].Task.ReviewComment == nil || *rows[1].Task.ReviewComment != comment {
			t.Fatalf("returned task must carry review_comment: %+v", rows[1])
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
			pgxmock.NewRows(taskCols).AddRow("team", "x", "not-a-number", "t", "d", "todo", nil, nil, nil))
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

var updCols = []string{"id", "title", "description", "status", "completed_at", "submitted_at", "review_comment"}

// expectCurrent — первый запрос UpdateStatus: текущий статус своей задачи.
func expectCurrent(db pgxmock.PgxPoolIface, taskID int64, status string) {
	db.ExpectQuery(`select status from buddy.intern_tasks`).WithArgs(taskID, int64(3)).
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(status))
}

func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("review sets submitted_at", func(t *testing.T) {
		db, p := newMock(t)
		expectCurrent(db, 7, "in_progress")
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "review").
			WillReturnRows(pgxmock.NewRows(updCols).AddRow(int64(7), "Доступ к GitLab", "описание", "review", nil, &now, nil))
		task, err := p.UpdateStatus(ctx, 3, 7, StatusReview)
		if err != nil || task.ID != 7 || task.Title != "Доступ к GitLab" ||
			task.Status != StatusReview || task.SubmittedAt == nil || task.CompletedAt != nil {
			t.Fatalf("review: %+v %v", task, err)
		}
	})

	t.Run("back to todo", func(t *testing.T) {
		db, p := newMock(t)
		expectCurrent(db, 7, "review")
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "todo").
			WillReturnRows(pgxmock.NewRows(updCols).AddRow(int64(7), "Доступ к GitLab", "описание", "todo", nil, nil, nil))
		task, err := p.UpdateStatus(ctx, 3, 7, StatusTodo)
		if err != nil || task.Status != StatusTodo || task.SubmittedAt != nil {
			t.Fatalf("todo: %+v %v", task, err)
		}
	})

	t.Run("foreign or missing task", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`select status from buddy.intern_tasks`).WithArgs(int64(99), int64(3)).WillReturnError(pgx.ErrNoRows)
		if _, err := p.UpdateStatus(ctx, 3, 99, StatusInProgress); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("already approved: no update", func(t *testing.T) {
		db, p := newMock(t)
		expectCurrent(db, 7, "done")
		if _, err := p.UpdateStatus(ctx, 3, 7, StatusInProgress); !errors.Is(err, ErrAlreadyApproved) {
			t.Fatalf("want ErrAlreadyApproved, got %v", err)
		}
	})

	t.Run("approved between queries", func(t *testing.T) {
		db, p := newMock(t)
		expectCurrent(db, 7, "review")
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "in_progress").WillReturnError(pgx.ErrNoRows)
		if _, err := p.UpdateStatus(ctx, 3, 7, StatusInProgress); !errors.Is(err, ErrAlreadyApproved) {
			t.Fatalf("want ErrAlreadyApproved, got %v", err)
		}
	})

	t.Run("db error on select", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`select status from buddy.intern_tasks`).WithArgs(int64(7), int64(3)).WillReturnError(errDB)
		_, err := p.UpdateStatus(ctx, 3, 7, StatusReview)
		if !errors.Is(err, errDB) || errors.Is(err, ErrNotFound) {
			t.Fatalf("want errDB (not ErrNotFound), got %v", err)
		}
	})

	t.Run("db error on update", func(t *testing.T) {
		db, p := newMock(t)
		expectCurrent(db, 7, "todo")
		db.ExpectQuery(`update buddy.intern_tasks`).WithArgs(int64(3), int64(7), "review").WillReturnError(errDB)
		_, err := p.UpdateStatus(ctx, 3, 7, StatusReview)
		if !errors.Is(err, errDB) || errors.Is(err, ErrAlreadyApproved) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

var reviewCols = []string{"task_id", "task_title", "task_description", "stage_title", "intern_id", "full_name", "submitted_at"}

func TestListReviews(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("rows", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`where u.mentor_id = \$1 and it.status = 'review'`).WithArgs(int64(1)).WillReturnRows(
			pgxmock.NewRows(reviewCols).
				AddRow(int64(17), "Доступ к Jira", "описание", "Доступы и инструменты", int64(3), "Саша Иванов", now))
		got, err := p.ListReviews(ctx, 1)
		if err != nil || len(got) != 1 {
			t.Fatalf("got %+v %v", got, err)
		}
		r := got[0]
		if r.TaskID != 17 || r.StageTitle != "Доступы и инструменты" || r.Intern != (Intern{ID: 3, FullName: "Саша Иванов"}) || !r.SubmittedAt.Equal(now) {
			t.Fatalf("review: %+v", r)
		}
	})

	t.Run("empty is not nil", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`it.status = 'review'`).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(reviewCols))
		got, err := p.ListReviews(ctx, 1)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("empty: %#v %v", got, err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`it.status = 'review'`).WithArgs(int64(1)).WillReturnError(errDB)
		if _, err := p.ListReviews(ctx, 1); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})

	t.Run("scan error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`it.status = 'review'`).WithArgs(int64(1)).WillReturnRows(
			pgxmock.NewRows(reviewCols).AddRow("bad", "t", "d", "s", int64(3), "n", now))
		if _, err := p.ListReviews(ctx, 1); err == nil {
			t.Fatal("want scan error")
		}
	})

	t.Run("rows.Err", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(`it.status = 'review'`).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(reviewCols).CloseError(errDB))
		if _, err := p.ListReviews(ctx, 1); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

// expectMiss — второй запрос approve/reject, когда update не тронул строк.
func expectMiss(db pgxmock.PgxPoolIface, status string, err error) {
	q := db.ExpectQuery(`select it.status from buddy.intern_tasks`).WithArgs(int64(17), int64(1))
	if err != nil {
		q.WillReturnError(err)
		return
	}
	q.WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(status))
}

var internCols = []string{"id", "full_name", "total", "done", "in_review", "stage"}

func TestListInterns(t *testing.T) {
	ctx := context.Background()
	const q = `left join buddy.intern_tasks it .+ where u.mentor_id = \$1`

	t.Run("rows", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(
			pgxmock.NewRows(internCols).
				AddRow(int64(13), "Дмитрий Орлов", 11, 0, 0, "Знакомство с командой").
				AddRow(int64(3), "Саша Иванов", 11, 5, 1, "Доступы и инструменты"))
		got, err := p.ListInterns(ctx, 1)
		if err != nil || len(got) != 2 {
			t.Fatalf("got %+v %v", got, err)
		}
		s := got[1]
		if s.Intern != (Intern{ID: 3, FullName: "Саша Иванов"}) || s.Total != 11 || s.Done != 5 || s.InReview != 1 || s.Stage != "Доступы и инструменты" {
			t.Fatalf("intern: %+v", s)
		}
		if s.Percent != 0 {
			t.Fatalf("percent is the service's job, store must leave 0: %d", s.Percent)
		}
	})

	t.Run("empty is not nil", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(internCols))
		got, err := p.ListInterns(ctx, 1)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("empty: %#v %v", got, err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(q).WithArgs(int64(1)).WillReturnError(errDB)
		if _, err := p.ListInterns(ctx, 1); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})

	t.Run("scan error", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(
			pgxmock.NewRows(internCols).AddRow("bad", "n", 1, 0, 0, ""))
		if _, err := p.ListInterns(ctx, 1); err == nil {
			t.Fatal("want scan error")
		}
	})

	t.Run("rows.Err", func(t *testing.T) {
		db, p := newMock(t)
		db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(internCols).CloseError(errDB))
		if _, err := p.ListInterns(ctx, 1); !errors.Is(err, errDB) {
			t.Fatalf("want errDB, got %v", err)
		}
	})
}

func TestApproveReject(t *testing.T) {
	ctx := context.Background()

	ops := []struct {
		name   string
		expect func(db pgxmock.PgxPoolIface) *pgxmock.ExpectedExec
		run    func(p *Postgres) error
	}{
		{"approve",
			func(db pgxmock.PgxPoolIface) *pgxmock.ExpectedExec {
				return db.ExpectExec(`set status = 'done'`).WithArgs(int64(1), int64(17))
			},
			func(p *Postgres) error { return p.Approve(ctx, 1, 17) }},
		{"reject",
			func(db pgxmock.PgxPoolIface) *pgxmock.ExpectedExec {
				return db.ExpectExec(`set status = 'in_progress'`).WithArgs(int64(1), "Нет доступа", int64(17))
			},
			func(p *Postgres) error { return p.Reject(ctx, 1, 17, "Нет доступа") }},
	}

	for _, op := range ops {
		t.Run(op.name+" ok", func(t *testing.T) {
			db, p := newMock(t)
			op.expect(db).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			if err := op.run(p); err != nil {
				t.Fatal(err)
			}
		})

		t.Run(op.name+" foreign or missing → not found", func(t *testing.T) {
			db, p := newMock(t)
			op.expect(db).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			expectMiss(db, "", pgx.ErrNoRows)
			if err := op.run(p); !errors.Is(err, ErrNotFound) {
				t.Fatalf("want ErrNotFound, got %v", err)
			}
		})

		t.Run(op.name+" not in review", func(t *testing.T) {
			db, p := newMock(t)
			op.expect(db).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			expectMiss(db, "done", nil)
			if err := op.run(p); !errors.Is(err, ErrNotInReview) {
				t.Fatalf("want ErrNotInReview, got %v", err)
			}
		})

		t.Run(op.name+" check error", func(t *testing.T) {
			db, p := newMock(t)
			op.expect(db).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			expectMiss(db, "", errDB)
			if err := op.run(p); !errors.Is(err, errDB) {
				t.Fatalf("want errDB, got %v", err)
			}
		})

		t.Run(op.name+" update error", func(t *testing.T) {
			db, p := newMock(t)
			op.expect(db).WillReturnError(errDB)
			if err := op.run(p); !errors.Is(err, errDB) {
				t.Fatalf("want errDB, got %v", err)
			}
		})
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusTodo, StatusInProgress, StatusReview, StatusDone} {
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
