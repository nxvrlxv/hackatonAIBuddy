package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// brokenTokens — TokenManager, который всегда падает.
type brokenTokens struct{}

func (brokenTokens) Issue(*auth.User) (string, error)   { return "", errors.New("sign failed") }
func (brokenTokens) Parse(string) (*auth.Claims, error) { return nil, errors.New("secret detail") }

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e myCustomErr
	if err := json.NewDecoder(rec.Body).Decode(&e); err != nil {
		t.Fatalf("error body must be json: %v", err)
	}
	return e.Error
}

func TestRequireAuthHidesParseError(t *testing.T) {
	h := New(fakeUsers{}, brokenTokens{}, nil, nil, testAgentKey).Handler()

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"no header", "", "missing bearer token"},
		{"wrong scheme", "Basic abc", "missing bearer token"},
		{"extra parts", "Bearer a b", "missing bearer token"},
		{"bad token", "Bearer abc", "invalid token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := decodeError(t, rec); got != tt.want {
				t.Fatalf("error = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginIssueFailure(t *testing.T) {
	users := fakeUsers{byLogin: map[string]*auth.User{"sasha.intern": sasha}}
	h := New(users, brokenTokens{}, nil, nil, testAgentKey).Handler()
	rec := do(t, h, http.MethodPost, "/api/login", `{"login":"sasha.intern"}`, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// Цепочка как у /internal/agent/*: requireAgentKey(requireAuth(h)).
func TestRequireAgentKey(t *testing.T) {
	_, manager := newTestServer(t, fakeUsers{})
	s := New(fakeUsers{}, manager, nil, nil, testAgentKey)
	token, err := manager.Issue(sasha)
	if err != nil {
		t.Fatal(err)
	}

	var seen *auth.Claims
	h := s.requireAgentKey(s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})))

	tests := []struct {
		name  string
		key   string
		token string
		want  int
		error string
	}{
		{"no key", "", token, http.StatusUnauthorized, "invalid agent key"},
		{"wrong key", "wrong-key", token, http.StatusUnauthorized, "invalid agent key"},
		{"key prefix", testAgentKey[:4], token, http.StatusUnauthorized, "invalid agent key"},
		{"key without token", testAgentKey, "", http.StatusUnauthorized, "missing bearer token"},
		{"all valid", testAgentKey, token, http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen = nil
			req := httptest.NewRequest(http.MethodGet, "/internal/agent/conversations", nil)
			if tt.key != "" {
				req.Header.Set("X-Agent-Key", tt.key)
			}
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if tt.want != http.StatusOK {
				if got := decodeError(t, rec); got != tt.error {
					t.Fatalf("error = %q, want %q", got, tt.error)
				}
				if seen != nil {
					t.Fatal("handler must not be called")
				}
				return
			}
			if seen == nil || seen.UserID != sasha.ID {
				t.Fatalf("handler must see the user, got %+v", seen)
			}
		})
	}
}

func TestRequireAgentKeyEmptyServerKey(t *testing.T) {
	// Сервер без ключа не пускает даже запрос с пустым X-Agent-Key.
	s := New(fakeUsers{}, nil, nil, nil, "")
	h := s.requireAgentKey(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/internal/agent/conversations", nil)
	req.Header.Set("X-Agent-Key", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
