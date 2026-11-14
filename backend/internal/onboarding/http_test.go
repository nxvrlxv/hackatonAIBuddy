package onboarding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// newHandler: wrap кладёт в контекст claims пользователя 3, как это делает requireAuth.
func newHandler(store *fakeStore, authed bool) http.Handler {
	mux := http.NewServeMux()
	wrap := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authed {
				r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{UserID: 3, TeamID: 2}))
			}
			next.ServeHTTP(w, r)
		})
	}
	NewHandlers(NewService(store)).Register(mux, wrap)
	return mux
}

func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestGetChecklistHTTP(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		store := &fakeStore{rows: sashaRows()}
		rec := call(newHandler(store, true), http.MethodGet, "/api/checklist", "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("status %d, ct %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		var c Checklist
		if err := json.NewDecoder(rec.Body).Decode(&c); err != nil {
			t.Fatal(err)
		}
		if c.Progress.Percent != 36 || c.Next == nil || c.Next.Title != "Доступ к GitLab" || len(c.Stages) != 3 {
			t.Fatalf("checklist: %+v", c)
		}
		if store.got.userID != 3 {
			t.Fatalf("user from claims must be used: %d", store.got.userID)
		}
	})

	t.Run("empty checklist is [] and null next", func(t *testing.T) {
		rec := call(newHandler(&fakeStore{}, true), http.MethodGet, "/api/checklist", "")
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, `"stages":[]`) || !strings.Contains(body, `"next":null`) {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		if rec := call(newHandler(&fakeStore{}, false), http.MethodGet, "/api/checklist", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})

	t.Run("store error", func(t *testing.T) {
		rec := call(newHandler(&fakeStore{ensureErr: errDB}, true), http.MethodGet, "/api/checklist", "")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "db down") {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})
}

func TestSetStatusHTTP(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rows := sashaRows()
		rows[4].Task.Status = StatusDone
		store := &fakeStore{rows: rows, task: Task{ID: 16, Title: "Доступ к GitLab", Status: StatusDone}}
		rec := call(newHandler(store, true), http.MethodPatch, "/api/checklist/tasks/16", `{"status":"done"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
		var res UpdateResult
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.Task.ID != 16 || res.Progress != (Progress{Done: 5, Total: 11, Percent: 45}) {
			t.Fatalf("result: %+v", res)
		}
		if store.got.userID != 3 || store.got.taskID != 16 || store.got.status != StatusDone {
			t.Fatalf("got %+v", store.got)
		}
	})

	cases := []struct {
		name   string
		store  *fakeStore
		authed bool
		path   string
		body   string
		want   int
	}{
		{"no claims", &fakeStore{}, false, "/api/checklist/tasks/16", `{"status":"done"}`, http.StatusUnauthorized},
		{"bad id", &fakeStore{}, true, "/api/checklist/tasks/abc", `{"status":"done"}`, http.StatusBadRequest},
		{"zero id", &fakeStore{}, true, "/api/checklist/tasks/0", `{"status":"done"}`, http.StatusBadRequest},
		{"bad json", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":`, http.StatusBadRequest},
		{"too large body", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":"` + strings.Repeat("x", 2048) + `"}`, http.StatusBadRequest},
		{"invalid status", &fakeStore{}, true, "/api/checklist/tasks/16", `{"status":"closed"}`, http.StatusBadRequest},
		{"foreign task", &fakeStore{updateErr: ErrNotFound}, true, "/api/checklist/tasks/99", `{"status":"done"}`, http.StatusNotFound},
		{"db error", &fakeStore{updateErr: errDB}, true, "/api/checklist/tasks/16", `{"status":"done"}`, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(newHandler(tc.store, tc.authed), http.MethodPatch, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
			var e map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e["error"] == "" {
				t.Fatalf("error body must be json: %s", rec.Body)
			}
		})
	}
}

func TestRegisterNilWrap(t *testing.T) {
	// Без wrap маршруты регистрируются, но без claims хендлер отвечает 401, а не паникует.
	mux := http.NewServeMux()
	NewHandlers(NewService(&fakeStore{})).Register(mux, nil)
	if rec := call(mux, http.MethodGet, "/api/checklist", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}
