package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// stubRoutes регистрирует одну ручку и отвечает 200, только если requireAuth положил claims.
type stubRoutes struct{}

func (stubRoutes) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	mux.Handle("GET /api/feature", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := auth.ClaimsFromContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		w.WriteHeader(http.StatusOK)
	})))
}

func TestWithRoutesMountsBehindAuth(t *testing.T) {
	manager, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := New(fakeUsers{}, manager, nil, nil, "key").WithRoutes(stubRoutes{}).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feature", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d, want 401", rec.Code)
	}

	token, err := manager.Issue(&auth.User{ID: 3, TeamID: 2})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/feature", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with token: %d, want 200", rec.Code)
	}
}
