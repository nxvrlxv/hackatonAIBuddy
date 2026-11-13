package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// fakeParser подменяет auth.Manager: возвращает заранее заданный результат.
type fakeParser struct {
	claims *auth.Claims
	err    error
}

func (f fakeParser) Parse(string) (*auth.Claims, error) { return f.claims, f.err }

func testClaims() *auth.Claims {
	return &auth.Claims{
		UserID: 5,
		TeamID: 3,
		Role:   "intern",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func TestVerifierValidToken(t *testing.T) {
	claims := testClaims()

	verifier := Verifier(fakeParser{claims: claims})

	info, err := verifier(context.Background(), "token", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UserID != "5" {
		t.Fatalf("UserID = %q, want %q", info.UserID, "5")
	}
	if !info.Expiration.Equal(claims.ExpiresAt.Time) {
		t.Fatalf("Expiration = %v, want %v", info.Expiration, claims.ExpiresAt.Time)
	}
	if info.Extra[claimsKey] != claims {
		t.Fatal("claims must be stored in TokenInfo.Extra")
	}
}

func TestVerifierInvalidToken(t *testing.T) {
	parser := fakeParser{err: auth.ErrInvalidToken}
	verifier := Verifier(parser)

	_, err := verifier(context.Background(), "token", nil)
	// Только ErrInvalidToken SDK превращает в 401; любая другая ошибка — 500.
	if !errors.Is(err, mcpauth.ErrInvalidToken) {
		t.Fatalf("err = %v, want mcpauth.ErrInvalidToken", err)
	}
}

func TestClaimsFromRequest(t *testing.T) {
	claims := testClaims()
	withExtra := func(extra map[string]any) *mcp.CallToolRequest {
		return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &mcpauth.TokenInfo{Extra: extra}}}
	}

	failures := map[string]*mcp.CallToolRequest{
		"nil request":    nil,
		"nil extra":      {},
		"nil token info": {Extra: &mcp.RequestExtra{}},
		"no claims":      withExtra(nil),
		"wrong type":     withExtra(map[string]any{claimsKey: "not claims"}),
		"nil claims ptr": withExtra(map[string]any{claimsKey: (*auth.Claims)(nil)}),
	}
	for name, req := range failures {
		t.Run(name, func(t *testing.T) {
			if _, err := ClaimsFromRequest(req); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("err = %v, want ErrUnauthenticated", err)
			}
		})
	}

	t.Run("ok", func(t *testing.T) {
		got, err := ClaimsFromRequest(withExtra(map[string]any{claimsKey: claims}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != claims {
			t.Fatal("must return the stored claims")
		}
	})
}

// Связка целиком: настоящий auth.Manager + middleware SDK, как в main.go.
func TestRequireBearerTokenIntegration(t *testing.T) {
	manager, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Issue(&auth.User{ID: 5, TeamID: 3, Role: "intern"})
	if err != nil {
		t.Fatal(err)
	}

	var seen *mcpauth.TokenInfo
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = mcpauth.TokenInfoFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := mcpauth.RequireBearerToken(Verifier(manager), nil)(next)

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"bad token", "Bearer garbage", http.StatusUnauthorized},
		{"valid token", "Bearer " + token, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen = nil
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
			if tt.want == http.StatusOK && (seen == nil || seen.UserID != "5") {
				t.Fatalf("handler must see the user, got %+v", seen)
			}
		})
	}
}
