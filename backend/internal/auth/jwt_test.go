package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-that-is-at-least-32-bytes-long"

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(testSecret, time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// sign подписывает произвольные claims любым методом — для «плохих» токенов.
func sign(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, key any) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return token
}

func TestNewmanagerValidation(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		ttl    time.Duration
	}{
		{
			name:   "short token",
			secret: "short token",
			ttl:    time.Hour,
		},
		{
			name:   "bad ttl",
			secret: testSecret,
			ttl:    0 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewManager(tt.secret, tt.ttl); err == nil {
				t.Fatal("expected error, found nil")
			}
		})
	}
}

func TestIssueParseRoundTrip(t *testing.T) {
	m := newTestManager(t)

	token, err := m.Issue(&User{ID: 5, TeamID: 3, Role: "intern"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if claims.UserID != 5 || claims.TeamID != 3 || claims.Role != "intern" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Subject != "5" {
		t.Fatalf("subject = %q, want %q", claims.Subject, "5")
	}
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) <= 0 {
		t.Fatalf("token must expire in the future, got %v", claims.ExpiresAt)
	}
}

func TestIssueNilUser(t *testing.T) {
	if _, err := newTestManager(t).Issue(nil); err == nil {
		t.Fatal("expected error for nil user")
	}
}

func TestParseRejectsInvalidTokens(t *testing.T) {
	m := newTestManager(t)
	valid := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}

	other, err := NewManager(strings.Repeat("x", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Issue(&User{ID: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Тест в том же пакете, поэтому можно собрать менеджер с отрицательным ttl напрямую.
	expiredManager := &Manager{secret: []byte(testSecret), ttl: -time.Minute}
	expired, err := expiredManager.Issue(&User{ID: 1})
	if err != nil {
		t.Fatal(err)
	}

	tokens := map[string]string{
		"empty":          "",
		"garbage":        "not-a-token",
		"three parts":    "a.b.c",
		"foreign secret": foreign,
		"expired":        expired,
		"alg none":       sign(t, jwt.SigningMethodNone, Claims{UserID: 1, RegisteredClaims: valid}, jwt.UnsafeAllowNoneSignatureType),
		"other alg":      sign(t, jwt.SigningMethodHS512, Claims{UserID: 1, RegisteredClaims: valid}, []byte(testSecret)),
		"no exp":         sign(t, jwt.SigningMethodHS256, Claims{UserID: 1}, []byte(testSecret)),
	}
	for name, token := range tokens {
		t.Run(name, func(t *testing.T) {
			claims, err := m.Parse(token)
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("err = %v, want ErrInvalidToken", err)
			}
			if claims != nil {
				t.Fatalf("claims must be nil on error, got %+v", claims)
			}
		})
	}
}
