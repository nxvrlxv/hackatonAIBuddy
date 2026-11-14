package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

// requestFor собирает запрос так же, как SDK после проверки Bearer-токена.
func requestFor(t *testing.T, userID int64) *mcp.CallToolRequest {
	t.Helper()
	m, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Issue(&auth.User{ID: userID, TeamID: 2, Role: "intern"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := mcpserver.Verifier(m)(context.Background(), token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &mcp.CallToolRequest{Extra: &mcp.RequestExtra{TokenInfo: info}}
}

func TestRegisterTools(t *testing.T) {
	// AddTool выводит JSON-схемы из типов и паникует на неподдерживаемых — проверяем, что не паникует.
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	RegisterTools(server, NewService(&fakeStore{}))
}

func TestGetChecklistTool(t *testing.T) {
	ctx := context.Background()

	t.Run("ok uses user from token", func(t *testing.T) {
		store := &fakeStore{rows: sashaRows()}
		_, out, err := getChecklist(NewService(store))(ctx, requestFor(t, 3), GetChecklistInput{})
		if err != nil || out.Progress.Total != 11 || out.Next == nil || out.Next.ID != 16 {
			t.Fatalf("out: %+v %v", out, err)
		}
		if store.got.userID != 3 {
			t.Fatalf("user from token must be used, got %d", store.got.userID)
		}
	})

	t.Run("no claims", func(t *testing.T) {
		store := &fakeStore{}
		_, _, err := getChecklist(NewService(store))(ctx, &mcp.CallToolRequest{}, GetChecklistInput{})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) || len(store.calls) != 0 {
			t.Fatalf("err %v, calls %v", err, store.calls)
		}
	})

	t.Run("store error is hidden", func(t *testing.T) {
		_, _, err := getChecklist(NewService(&fakeStore{listErr: errDB}))(ctx, requestFor(t, 3), GetChecklistInput{})
		if err == nil || errors.Is(err, errDB) || strings.Contains(err.Error(), "db down") {
			t.Fatalf("internal error must not leak to the model: %v", err)
		}
	})
}

func TestUpdateTaskStatusTool(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		rows := sashaRows()
		rows[4].Task.Status = StatusDone
		store := &fakeStore{rows: rows, task: Task{ID: 16, Title: "Доступ к GitLab", Status: StatusDone}}
		_, out, err := updateTaskStatus(NewService(store))(ctx, requestFor(t, 3),
			UpdateTaskStatusInput{TaskID: 16, Status: "done"})
		if err != nil || out.Task.ID != 16 || out.Progress.Done != 5 {
			t.Fatalf("out: %+v %v", out, err)
		}
		if store.got.userID != 3 || store.got.taskID != 16 || store.got.status != StatusDone {
			t.Fatalf("got %+v", store.got)
		}
	})

	cases := []struct {
		name  string
		store *fakeStore
		in    UpdateTaskStatusInput
		want  string
	}{
		{"invalid status", &fakeStore{}, UpdateTaskStatusInput{TaskID: 16, Status: "closed"}, "неверный статус"},
		{"not found", &fakeStore{updateErr: ErrNotFound}, UpdateTaskStatusInput{TaskID: 99, Status: "done"}, "задача не найдена"},
		{"db error", &fakeStore{updateErr: errDB}, UpdateTaskStatusInput{TaskID: 16, Status: "done"}, "не удалось обновить"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := updateTaskStatus(NewService(tc.store))(ctx, requestFor(t, 3), tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "db down") {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("no claims", func(t *testing.T) {
		store := &fakeStore{}
		_, _, err := updateTaskStatus(NewService(store))(ctx, nil, UpdateTaskStatusInput{TaskID: 1, Status: "done"})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) || len(store.calls) != 0 {
			t.Fatalf("err %v, calls %v", err, store.calls)
		}
	})
}
