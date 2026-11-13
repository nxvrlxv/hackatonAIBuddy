package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// fakeUsers — репозиторий в памяти. err, если задан, возвращается из любого метода (сбой базы).
type fakeUsers struct {
	byLogin map[string]*auth.User
	err     error
}

func (f fakeUsers) GetUserByLogin(_ context.Context, login string) (*auth.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.byLogin[login]; ok {
		return u, nil
	}
	return nil, auth.ErrNotFound
}

func (f fakeUsers) GetUserByID(_ context.Context, id int64) (*auth.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, u := range f.byLogin {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, auth.ErrNotFound
}

const testAgentKey = "test-agent-key"

var sasha = &auth.User{ID: 5, Login: "sasha.intern", FullName: "Саша Иванов", Role: "intern", TeamID: 3, Team: "travel"}

func newTestServer(t *testing.T, users UserStorage) (http.Handler, *auth.Manager) {
	t.Helper()
	manager, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return New(users, manager, nil, nil, testAgentKey).Handler(), manager
}

func do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLogin(t *testing.T) {
	users := fakeUsers{byLogin: map[string]*auth.User{"sasha.intern": sasha}}

	tests := []struct {
		name  string
		users UserStorage
		body  string
		want  int
	}{
		{"ok", users, `{"login":"sasha.intern"}`, http.StatusOK},
		{"ok with spaces", users, `{"login":"  sasha.intern "}`, http.StatusOK},
		{"unknown login", users, `{"login":"nobody"}`, http.StatusUnauthorized},
		{"empty login", users, `{"login":"   "}`, http.StatusBadRequest},
		{"broken json", users, `{"login":`, http.StatusBadRequest},
		{"db error", fakeUsers{err: errors.New("db down")}, `{"login":"sasha.intern"}`, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestServer(t, tt.users)
			rec := do(t, h, http.MethodPost, "/api/login", tt.body, "")
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestLoginReturnsWorkingToken(t *testing.T) {
	h, manager := newTestServer(t, fakeUsers{byLogin: map[string]*auth.User{"sasha.intern": sasha}})

	rec := do(t, h, http.MethodPost, "/api/login", `{"login":"sasha.intern"}`, "")
	var resp loginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.FullName != "Саша Иванов" || resp.User.TeamID != 3 {
		t.Fatalf("unexpected user: %+v", resp.User)
	}
	claims, err := manager.Parse(resp.Token)
	if err != nil {
		t.Fatalf("token from login must be valid: %v", err)
	}
	if claims.UserID != 5 || claims.TeamID != 3 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestMe(t *testing.T) {
	users := fakeUsers{byLogin: map[string]*auth.User{"sasha.intern": sasha}}
	h, manager := newTestServer(t, users)

	token, err := manager.Issue(sasha)
	if err != nil {
		t.Fatal(err)
	}
	ghostToken, err := manager.Issue(&auth.User{ID: 999})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/me", "", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var got userResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Login != "sasha.intern" {
			t.Fatalf("unexpected user: %+v", got)
		}
	})

	failures := []struct {
		name  string
		h     http.Handler
		token string
		want  int
	}{
		{"no token", h, "", http.StatusUnauthorized},
		{"bad token", h, "garbage", http.StatusUnauthorized},
		{"user deleted", h, ghostToken, http.StatusUnauthorized},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			if rec := do(t, tt.h, http.MethodGet, "/api/me", "", tt.token); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}

	t.Run("db error", func(t *testing.T) {
		broken, m := newTestServer(t, fakeUsers{err: errors.New("db down")})
		tok, err := m.Issue(sasha)
		if err != nil {
			t.Fatal(err)
		}
		if rec := do(t, broken, http.MethodGet, "/api/me", "", tok); rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})
}

func TestMeWithoutMiddleware(t *testing.T) {
	// Защита от ошибки маршрутизации: хендлер без requireAuth не должен паниковать.
	s := New(fakeUsers{}, nil, nil, nil, testAgentKey)
	rec := httptest.NewRecorder()
	s.handleMe(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCORSAndHealth(t *testing.T) {
	h, _ := newTestServer(t, fakeUsers{})

	t.Run("preflight without token", func(t *testing.T) {
		rec := do(t, h, http.MethodOptions, "/api/me", "", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatal("missing CORS header")
		}
	})
	t.Run("healthz", func(t *testing.T) {
		if rec := do(t, h, http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})
	t.Run("errors are json", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/me", "", "")
		var e myCustomErr
		if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e.Error == "" {
			t.Fatalf("error body must be json, got %q", rec.Body)
		}
	})
}
