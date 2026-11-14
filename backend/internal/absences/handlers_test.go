package absences

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

func newMux(s *Service, authed bool) http.Handler {
	mux := http.NewServeMux()
	s.Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authed {
				r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{UserID: 3, TeamID: 2}))
			}
			next.ServeHTTP(w, r)
		})
	})
	return mux
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e["error"] == "" {
		t.Fatalf("error body must be json {error}: %s", rec.Body)
	}
	return e["error"]
}

func TestHTTP(t *testing.T) {
	const body = `{"start_date":"2026-11-20","end_date":"2026-11-20","idempotency_key":"k1"}`

	t.Run("types", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`request_types`).WillReturnRows(pgxmock.NewRows([]string{"code", "title", "description", "fields"}).
			AddRow("time_off", "Отгул", "d", map[string]any{}))
		rec := do(newMux(s, true), http.MethodGet, "/api/request-types", "")
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || !strings.Contains(rec.Body.String(), "time_off") {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})

	t.Run("list", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnRows(sampleRow())
		rec := do(newMux(s, true), http.MethodGet, "/api/requests", "")
		var v []Request
		if rec.Code != 200 || json.NewDecoder(rec.Body).Decode(&v) != nil || len(v) != 1 {
			t.Fatalf("%d %+v", rec.Code, v)
		}
	})

	t.Run("create", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(`INSERT`).WithArgs(int64(3), pgxmock.AnyArg(), "k1").WillReturnRows(sampleRow())
		rec := do(newMux(s, true), http.MethodPost, "/api/requests/time-off", body)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"submitted"`) {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
	})

	t.Run("invalid dates → 422", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnError(pgx.ErrNoRows)
		rec := do(newMux(s, true), http.MethodPost, "/api/requests/time-off",
			`{"start_date":"2020-01-01","end_date":"2020-01-01","idempotency_key":"k1"}`)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(errorOf(t, rec), "даты") {
			t.Fatalf("%d", rec.Code)
		}
	})

	t.Run("conflict → 409", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnRows(sampleRow())
		db.ExpectQuery(`SELECT data=`).WithArgs(int64(3), "k1", pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"same"}).AddRow(false))
		rec := do(newMux(s, true), http.MethodPost, "/api/requests/time-off", body)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%d", rec.Code)
		}
		errorOf(t, rec)
	})

	t.Run("db error → 500 hidden", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnError(errDB)
		rec := do(newMux(s, true), http.MethodGet, "/api/requests", "")
		if rec.Code != 500 || strings.Contains(errorOf(t, rec), "db down") {
			t.Fatalf("%d", rec.Code)
		}
	})

	for _, tc := range []struct{ name, body string }{
		{"broken json", `{"start_date":`},
		{"unknown field", `{"start_date":"2026-11-20","end_date":"2026-11-20","idempotency_key":"k1","user_id":1}`},
	} {
		t.Run(tc.name+" → 400", func(t *testing.T) {
			_, s := newMock(t)
			rec := do(newMux(s, true), http.MethodPost, "/api/requests/time-off", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%d", rec.Code)
			}
			errorOf(t, rec)
		})
	}

	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/request-types"},
		{http.MethodGet, "/api/requests"},
		{http.MethodPost, "/api/requests/time-off"},
	} {
		t.Run("no claims "+r.path, func(t *testing.T) {
			_, s := newMock(t)
			rec := do(newMux(s, false), r.method, r.path, body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%d", rec.Code)
			}
			errorOf(t, rec)
		})
	}

	t.Run("nil wrap", func(t *testing.T) {
		_, s := newMock(t)
		mux := http.NewServeMux()
		s.Register(mux, nil)
		if rec := do(mux, http.MethodGet, "/api/requests", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%d", rec.Code)
		}
	})
}

func requestFor(t *testing.T, userID int64) *mcp.CallToolRequest {
	t.Helper()
	m, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Issue(&auth.User{ID: userID, TeamID: 2})
	if err != nil {
		t.Fatal(err)
	}
	info, err := mcpserver.Verifier(m)(context.Background(), token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: info}}
}

func TestMCP(t *testing.T) {
	ctx := context.Background()

	t.Run("register does not panic", func(t *testing.T) {
		_, s := newMock(t)
		Register(mcp.NewServer(&mcp.Implementation{Name: "t", Version: "v0"}, nil), s)
	})

	t.Run("types", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`request_types`).WillReturnRows(pgxmock.NewRows([]string{"code", "title", "description", "fields"}).
			AddRow("time_off", "Отгул", "d", map[string]any{}))
		_, out, err := getTypes(s)(ctx, requestFor(t, 3), Empty{})
		if err != nil || len(out.Types) != 1 {
			t.Fatalf("%+v %v", out, err)
		}
	})

	t.Run("list uses user from token", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(5)).WillReturnRows(sampleRow())
		_, out, err := listMine(s)(ctx, requestFor(t, 5), Empty{})
		if err != nil || len(out.Requests) != 1 {
			t.Fatalf("%+v %v", out, err)
		}
	})

	t.Run("create", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(`INSERT`).WithArgs(int64(3), pgxmock.AnyArg(), "k1").WillReturnRows(sampleRow())
		_, out, err := createTimeOff(s)(ctx, requestFor(t, 3),
			Input{StartDate: "2026-11-20", EndDate: "2026-11-20", IdempotencyKey: "k1"})
		if err != nil || out.ID != 7 {
			t.Fatalf("%+v %v", out, err)
		}
	})

	t.Run("invalid input is shown to the model", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`idempotency_key=\$2`).WithArgs(int64(3), "k1").WillReturnError(pgx.ErrNoRows)
		_, _, err := createTimeOff(s)(ctx, requestFor(t, 3), Input{StartDate: "2020-01-01", EndDate: "2020-01-01", IdempotencyKey: "k1"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	})

	t.Run("db error is hidden", func(t *testing.T) {
		db, s := newMock(t)
		db.ExpectQuery(`FROM buddy.requests`).WithArgs(int64(3)).WillReturnError(errDB)
		_, _, err := listMine(s)(ctx, requestFor(t, 3), Empty{})
		if err == nil || errors.Is(err, errDB) || strings.Contains(err.Error(), "db down") {
			t.Fatal(err)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		_, s := newMock(t)
		empty := &mcp.CallToolRequest{}
		if _, _, err := getTypes(s)(ctx, empty, Empty{}); !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, _, err := listMine(s)(ctx, empty, Empty{}); !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatal(err)
		}
		if _, _, err := createTimeOff(s)(ctx, empty, Input{}); !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
}
