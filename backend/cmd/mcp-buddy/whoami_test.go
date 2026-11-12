package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

// fakeUsers подменяет репозиторий: отдаёт пользователя по id или err.
type fakeUsers struct {
	users map[int64]*auth.User
	err   error
}

func (f fakeUsers) GetUserByID(_ context.Context, id int64) (*auth.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return nil, auth.ErrNotFound
}

// requestFor собирает запрос так же, как его собирает SDK после проверки токена.
func requestFor(t *testing.T, user *auth.User) *mcp.CallToolRequest {
	t.Helper()
	m, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Issue(user)
	if err != nil {
		t.Fatal(err)
	}
	info, err := mcpserver.Verifier(m)(context.Background(), token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: info}}
}

func TestWhoAmI(t *testing.T) {
	mentor := "Анна Смирнова"
	sasha := &auth.User{ID: 5, Login: "sasha.intern", FullName: "Саша Иванов", Role: "intern", TeamID: 3, Team: "Команда Авито Путешествий", Mentor: &mentor}
	handler := whoami(fakeUsers{users: map[int64]*auth.User{5: sasha}})

	_, out, err := handler(context.Background(), requestFor(t, sasha), WhoAmIInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := WhoAmIOutput{ID: 5, FullName: "Саша Иванов", Role: "intern", TeamID: 3, Team: "Команда Авито Путешествий", Mentor: &mentor}
	if out.ID != want.ID || out.FullName != want.FullName || out.Role != want.Role ||
		out.TeamID != want.TeamID || out.Team != want.Team || out.Mentor == nil || *out.Mentor != mentor {
		t.Fatalf("out = %+v, want %+v", out, want)
	}

	// агент читает snake_case
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "full_name", "role", "team_id", "team", "mentor"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("json must contain %q: %s", key, raw)
		}
	}
}

func TestWhoAmIErrors(t *testing.T) {
	ghost := &auth.User{ID: 999}

	t.Run("no token info", func(t *testing.T) {
		_, _, err := whoami(fakeUsers{})(context.Background(), &mcp.CallToolRequest{}, WhoAmIInput{})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})
	t.Run("empty token info", func(t *testing.T) {
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: &mcpauth.TokenInfo{}}}
		_, _, err := whoami(fakeUsers{})(context.Background(), req, WhoAmIInput{})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatalf("err = %v, want ErrUnauthenticated", err)
		}
	})
	t.Run("user not found", func(t *testing.T) {
		_, _, err := whoami(fakeUsers{})(context.Background(), requestFor(t, ghost), WhoAmIInput{})
		if !errors.Is(err, auth.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("db error", func(t *testing.T) {
		dbErr := errors.New("db down")
		_, _, err := whoami(fakeUsers{err: dbErr})(context.Background(), requestFor(t, ghost), WhoAmIInput{})
		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v, want db error", err)
		}
	})
}

func TestRegisterTools(t *testing.T) {
	// Регистрация не должна паниковать (SDK строит JSON-схемы из типов).
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	RegisterServerTools(server)
	RegisterWhoAmI(server, fakeUsers{})
	RegisterSearchDocs(server, nil)
}

func TestPing(t *testing.T) {
	_, out, err := ping(context.Background(), nil, PingInput{})
	if err != nil || out.Status != "ok" || out.Name != "avito-buddy" {
		t.Fatalf("ping = %+v, %v", out, err)
	}
}
