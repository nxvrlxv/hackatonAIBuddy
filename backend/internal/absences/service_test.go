package absences

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

var (
	errDB    = errors.New("db down")
	fixedNow = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	reqCols  = []string{"id", "type_code", "data", "status", "created_at"}
)

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, start, end, reason, key string
		bad                           bool
	}{
		{"valid", "2026-11-13", "2026-11-14", "", "a", false},
		{"one day", "2026-11-20", "2026-11-20", "", "a", false},
		{"max length", "2026-11-20", "2026-12-19", "", "a", false},
		{"too long", "2026-11-20", "2026-12-20", "", "a", true},
		{"past", "2026-11-12", "2026-11-14", "", "a", true},
		{"reversed", "2026-11-15", "2026-11-14", "", "a", true},
		{"invalid date", "2026-02-30", "2026-11-14", "", "a", true},
		{"missing key", "2026-11-13", "2026-11-14", "", "", true},
		{"blank key", "2026-11-13", "2026-11-14", "", "   ", true},
		{"key too long", "2026-11-13", "2026-11-14", "", strings.Repeat("k", 201), true},
		{"reason too long", "2026-11-13", "2026-11-14", strings.Repeat("я", 2001), "a", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(Input{StartDate: tc.start, EndDate: tc.end, Reason: tc.reason, IdempotencyKey: tc.key}, fixedNow)
			if (err != nil) != tc.bad || (err != nil && !errors.Is(err, ErrInvalid)) {
				t.Fatalf("err = %v, bad = %v", err, tc.bad)
			}
		})
	}
}

func TestValidationUsesMoscowDate(t *testing.T) {
	// 22:30 UTC 13.11 — это уже 14.11 в Москве: 13-е стало прошлым.
	late := time.Date(2026, 11, 13, 22, 30, 0, 0, time.UTC)
	if err := validate(Input{StartDate: "2026-11-13", EndDate: "2026-11-13", IdempotencyKey: "a"}, late); !errors.Is(err, ErrInvalid) {
		t.Fatalf("13.11 must be in the past by Moscow time: %v", err)
	}
}

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Service) {
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
	s := New(db)
	s.now = func() time.Time { return fixedNow }
	return db, s
}

func sampleRow() *pgxmock.Rows {
	return pgxmock.NewRows(reqCols).AddRow(int64(7), "time_off",
		map[string]any{"start_date": "2026-11-20", "end_date": "2026-11-20", "reason": ""}, "submitted", fixedNow)
}

func TestTypes(t *testing.T) {
	ctx := context.Background()
	cols := []string{"code", "title", "description", "fields"}

	t.Run("ok", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.request_types WHERE enabled`).WillReturnRows(pgxmock.NewRows(cols).
			AddRow("time_off", "Отгул", "описание", map[string]any{"start_date": map[string]any{"type": "date"}}))
		v, err := s.Types(ctx)
		if err != nil || len(v) != 1 || v[0].Code != "time_off" || v[0].Fields["start_date"] == nil {
			t.Fatalf("%+v %v", v, err)
		}
	})
	t.Run("query error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`request_types`).WillReturnError(errDB)
		if _, err := s.Types(ctx); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})
	t.Run("scan error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`request_types`).WillReturnRows(pgxmock.NewRows(cols).AddRow("a", "b", "c", "not-a-map"))
		if _, err := s.Types(ctx); err == nil {
			t.Fatal("want scan error")
		}
	})
}

func TestList(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests WHERE user_id=\$1`).WithArgs(int64(3)).WillReturnRows(sampleRow())
		v, err := s.List(ctx, 3)
		if err != nil || len(v) != 1 || v[0].ID != 7 || v[0].Status != "submitted" {
			t.Fatalf("%+v %v", v, err)
		}
	})
	t.Run("empty is not nil", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnRows(pgxmock.NewRows(reqCols))
		if v, err := s.List(ctx, 3); err != nil || v == nil || len(v) != 0 {
			t.Fatalf("%#v %v", v, err)
		}
	})
	t.Run("query error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnError(errDB)
		if _, err := s.List(ctx, 3); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})
	t.Run("scan error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnRows(
			pgxmock.NewRows(reqCols).AddRow("x", "time_off", map[string]any{}, "submitted", fixedNow))
		if _, err := s.List(ctx, 3); err == nil {
			t.Fatal("want scan error")
		}
	})
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	in := Input{StartDate: "2026-11-20", EndDate: "2026-11-20", Reason: "  семейные дела  ", IdempotencyKey: "time-off:run-1"}
	const (
		lookup  = `SELECT id,type_code,data,status,created_at FROM buddy.requests WHERE user_id=\$1 AND idempotency_key=\$2`
		compare = `SELECT data=\$3::jsonb`
		insert  = `INSERT INTO buddy.requests`
		exists  = `SELECT EXISTS`
	)
	any := pgxmock.AnyArg()

	t.Run("new request", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnRows(sampleRow())
		v, err := s.Create(ctx, 3, in)
		if err != nil || v.ID != 7 {
			t.Fatalf("%+v %v", v, err)
		}
	})

	t.Run("identical replay returns original", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(sampleRow())
		db.ExpectQuery(compare).WithArgs(int64(3), in.IdempotencyKey, any).WillReturnRows(pgxmock.NewRows([]string{"same"}).AddRow(true))
		if v, err := s.Create(ctx, 3, in); err != nil || v.ID != 7 {
			t.Fatalf("%+v %v", v, err)
		}
	})

	t.Run("same key other data", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(sampleRow())
		db.ExpectQuery(compare).WithArgs(int64(3), in.IdempotencyKey, any).WillReturnRows(pgxmock.NewRows([]string{"same"}).AddRow(false))
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})

	t.Run("compare error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(sampleRow())
		db.ExpectQuery(compare).WithArgs(int64(3), in.IdempotencyKey, any).WillReturnError(errDB)
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})

	t.Run("lookup error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(errDB)
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})

	t.Run("invalid input is not inserted", func(t *testing.T) {
		db, s := newMock(t)
		bad := in
		bad.StartDate = "2026-11-01"
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		if _, err := s.Create(ctx, 3, bad); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	})

	t.Run("type disabled", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	})

	t.Run("same dates already requested", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnError(&pgconn.PgError{Code: "23505"})
		db.ExpectQuery(exists).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
		_, err := s.Create(ctx, 3, in)
		if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "эти даты") {
			t.Fatal(err)
		}
	})

	t.Run("concurrent replay resolves to original", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnError(&pgconn.PgError{Code: "23505"})
		db.ExpectQuery(exists).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnRows(sampleRow())
		db.ExpectQuery(compare).WithArgs(int64(3), in.IdempotencyKey, any).WillReturnRows(pgxmock.NewRows([]string{"same"}).AddRow(true))
		if v, err := s.Create(ctx, 3, in); err != nil || v.ID != 7 {
			t.Fatalf("%+v %v", v, err)
		}
	})

	t.Run("exists check error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnError(&pgconn.PgError{Code: "23505"})
		db.ExpectQuery(exists).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(errDB)
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})

	t.Run("insert error", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(lookup).WithArgs(int64(3), in.IdempotencyKey).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(insert).WithArgs(int64(3), any, in.IdempotencyKey).WillReturnError(errDB)
		if _, err := s.Create(ctx, 3, in); !errors.Is(err, errDB) {
			t.Fatal(err)
		}
	})
}

func TestCreateOneDayDefaultsEndDate(t *testing.T) {
	db, s := newMock(t)
	db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnError(pgx.ErrNoRows)
	// В JSONB уходит end_date = start_date.
	db.ExpectQuery(`INSERT`).WithArgs(int64(3), []byte(`{"end_date":"2026-11-20","reason":"","start_date":"2026-11-20"}`), "k1").
		WillReturnRows(sampleRow())
	if _, err := s.Create(context.Background(), 3, Input{StartDate: "2026-11-20", IdempotencyKey: "k1"}); err != nil {
		t.Fatal(err)
	}
}
