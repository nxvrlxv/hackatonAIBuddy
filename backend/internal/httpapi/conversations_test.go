package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
)

// convChat — ChatService для публичных ручек: отдаёт msgs и err, запоминает аргументы.
type convChat struct {
	msgs   []chat.Message
	err    error
	userID int64
	title  string
	listIn chat.ListMessagesInput
}

func (f *convChat) CreateConversation(_ context.Context, userID int64, title string) (chat.Conversation, error) {
	f.userID, f.title = userID, title
	return chat.Conversation{ID: 7, Title: title}, f.err
}

func (f *convChat) ListConversations(_ context.Context, userID int64) ([]chat.Conversation, error) {
	f.userID = userID
	return []chat.Conversation{{ID: 7}, {ID: 8}}, f.err
}

func (f *convChat) ListMessages(_ context.Context, in chat.ListMessagesInput) ([]chat.Message, error) {
	f.listIn = in
	return f.msgs, f.err
}

func (f *convChat) StartTurn(context.Context, chat.StartTurnInput) (chat.Turn, error) {
	return chat.Turn{RunID: "run"}, f.err
}

func (f *convChat) AppendMessage(context.Context, chat.AppendInput) (chat.Message, bool, error) {
	return chat.Message{}, true, f.err
}

func (f *convChat) FinishRun(context.Context, int64, int64, string, string) (chat.Run, error) {
	return chat.Run{}, f.err
}

// stubAgent — вместо прокси: отвечает 200 и запоминает, что до него дошли.
type stubAgent struct{ hits int }

func (s *stubAgent) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.hits++
	w.WriteHeader(http.StatusOK)
}

func newFullServer(t *testing.T, svc ChatService, agent http.Handler) (http.Handler, string) {
	t.Helper()
	manager, err := auth.NewManager("test-secret-that-is-at-least-32-bytes-long", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Issue(sasha)
	if err != nil {
		t.Fatal(err)
	}
	return New(fakeUsers{}, manager, svc, agent, testAgentKey).Handler(), token
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return body
}

func msg(id int64, role, payload string) chat.Message {
	return chat.Message{ID: id, Role: role, SequenceNumber: id, Payload: json.RawMessage(payload)}
}

func TestListMessagesVisibleTextOnly(t *testing.T) {
	svc := &convChat{msgs: []chat.Message{
		msg(1, chat.RoleSystem, `{"role":"system","content":"секретный промпт"}`),
		msg(2, chat.RoleUser, `{"role":"user","content":"Где отпуск?"}`),
		msg(3, chat.RoleAssistant, `{"role":"assistant","content":null,"tool_calls":[{"id":"c1"}]}`),
		msg(4, chat.RoleTool, `{"role":"tool","content":"сырые данные"}`),
		msg(5, chat.RoleAssistant, `{"role":"assistant","content":"   "}`),
		msg(6, chat.RoleAssistant, `"просто строка"`),
		msg(7, chat.RoleAssistant, `not json`),
		msg(8, chat.RoleAssistant, `{"role":"assistant"}`),
		msg(9, chat.RoleAssistant, `{"content":[{"type":"text","text":"часть 1 "},{"type":"image_url"},{"type":"text","text":"часть 2"}]}`),
		msg(10, chat.RoleAssistant, `{"content":{"weird":true}}`),
		msg(11, chat.RoleAssistant, `{"role":"assistant","content":"Отпуск — в портале.","tool_calls":[]}`),
	}}
	h, token := newFullServer(t, svc, &stubAgent{})

	rec := do(t, h, http.MethodGet, "/api/conversations/3/messages?limit=20&before=100", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if svc.listIn != (chat.ListMessagesInput{UserID: sasha.ID, ConversationID: 3, Limit: 20, Before: 100}) {
		t.Fatalf("list input = %+v", svc.listIn)
	}

	var body struct {
		Messages []visibleMessage `json:"messages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(body.Messages))
	for _, m := range body.Messages {
		got = append(got, fmt.Sprintf("%d:%s:%s", m.ID, m.Role, m.Content))
	}
	want := []string{"2:user:Где отпуск?", "9:assistant:часть 1 часть 2", "11:assistant:Отпуск — в портале."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("messages = %q, want %q", got, want)
	}
	if body.Messages[0].SequenceNumber != 2 {
		t.Fatalf("sequence_number = %d", body.Messages[0].SequenceNumber)
	}
}

func TestListMessagesEmptyIsArray(t *testing.T) {
	h, token := newFullServer(t, &convChat{}, &stubAgent{})
	rec := do(t, h, http.MethodGet, "/api/conversations/3/messages", "", token)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"messages":[]}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestConversationsCreateAndList(t *testing.T) {
	svc := &convChat{}
	h, token := newFullServer(t, svc, &stubAgent{})

	rec := do(t, h, http.MethodPost, "/api/conversations", `{"title":"Отпуск"}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	if body := decodeBody(t, rec); body["id"] != float64(7) || body["title"] != "Отпуск" {
		t.Fatalf("create body = %v", body)
	}
	if svc.userID != sasha.ID || svc.title != "Отпуск" {
		t.Fatalf("service got user %d title %q", svc.userID, svc.title)
	}

	// Пустое тело допустимо.
	if rec := do(t, h, http.MethodPost, "/api/conversations", "", token); rec.Code != http.StatusCreated {
		t.Fatalf("empty body status = %d", rec.Code)
	}

	rec = do(t, h, http.MethodGet, "/api/conversations", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	if list, _ := decodeBody(t, rec)["conversations"].([]any); len(list) != 2 {
		t.Fatalf("conversations = %v", list)
	}
}

func TestConversationsErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		method string
		path   string
		body   string
		want   int
		msg    string
	}{
		{"create invalid json", nil, http.MethodPost, "/api/conversations", `{`, http.StatusBadRequest, "invalid json"},
		{"create invalid input", fmt.Errorf("%w: title too long", chat.ErrInvalidInput), http.MethodPost, "/api/conversations", `{}`, http.StatusBadRequest, "title too long"},
		{"create db error", errors.New("db down"), http.MethodPost, "/api/conversations", `{}`, http.StatusInternalServerError, "internal error"},
		{"list db error", errors.New("db down"), http.MethodGet, "/api/conversations", "", http.StatusInternalServerError, "internal error"},
		{"messages not found", chat.ErrNotFound, http.MethodGet, "/api/conversations/3/messages", "", http.StatusNotFound, "not found"},
		{"messages db error", errors.New("db down"), http.MethodGet, "/api/conversations/3/messages", "", http.StatusInternalServerError, "internal error"},
		{"messages bad id", nil, http.MethodGet, "/api/conversations/abc/messages", "", http.StatusBadRequest, "invalid conversation id"},
		{"messages zero id", nil, http.MethodGet, "/api/conversations/0/messages", "", http.StatusBadRequest, "invalid conversation id"},
		{"messages bad limit", nil, http.MethodGet, "/api/conversations/3/messages?limit=x", "", http.StatusBadRequest, "invalid limit"},
		{"messages bad before", nil, http.MethodGet, "/api/conversations/3/messages?before=x", "", http.StatusBadRequest, "invalid before"},
		{"messages invalid input", fmt.Errorf("%w: limit out of range", chat.ErrInvalidInput), http.MethodGet, "/api/conversations/3/messages?limit=999", "", http.StatusBadRequest, "limit out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, token := newFullServer(t, &convChat{err: tt.err}, &stubAgent{})
			rec := do(t, h, tt.method, tt.path, tt.body, token)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if got := decodeBody(t, rec)["error"]; got != tt.msg {
				t.Fatalf("error = %v, want %q", got, tt.msg)
			}
		})
	}
}

func TestConversationHandlersWithoutClaims(t *testing.T) {
	// Защита на случай, если маршрут забудут обернуть в requireAuth.
	s := New(fakeUsers{}, nil, &convChat{}, nil, testAgentKey)
	for name, h := range map[string]http.HandlerFunc{
		"create":   s.handleCreateConversation,
		"list":     s.handleListConversations,
		"messages": s.handleListMessages,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d", name, rec.Code)
		}
	}
}

func TestRoutingAuth(t *testing.T) {
	agent := &stubAgent{}
	h, token := newFullServer(t, &convChat{}, agent)

	public := []struct{ method, path string }{
		{http.MethodPost, "/api/conversations"},
		{http.MethodGet, "/api/conversations"},
		{http.MethodGet, "/api/conversations/3/messages"},
		{http.MethodPost, "/api/chat"},
	}
	for _, r := range public {
		if rec := do(t, h, r.method, r.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: %d", r.method, r.path, rec.Code)
		}
	}
	if agent.hits != 0 {
		t.Fatal("proxy reached without token")
	}

	// /api/chat с токеном доходит до прокси.
	if rec := do(t, h, http.MethodPost, "/api/chat", `{}`, token); rec.Code != http.StatusOK || agent.hits != 1 {
		t.Fatalf("chat with token: %d, hits %d", rec.Code, agent.hits)
	}

	// Preflight проходит без токена и не доходит до прокси.
	pre := httptest.NewRequest(http.MethodOptions, "/api/chat", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != http.StatusNoContent || agent.hits != 1 {
		t.Fatalf("preflight: %d, hits %d", rec.Code, agent.hits)
	}
}

func TestRoutingInternalAgentNeedsKeyAndToken(t *testing.T) {
	h, token := newFullServer(t, &convChat{}, &stubAgent{})
	tests := []struct {
		name, key, token string
		want             int
	}{
		{"nothing", "", "", http.StatusUnauthorized},
		{"token only", "", token, http.StatusUnauthorized},
		{"wrong key", "wrong", token, http.StatusUnauthorized},
		{"key only", testAgentKey, "", http.StatusUnauthorized},
		{"key and bad token", testAgentKey, "garbage", http.StatusUnauthorized},
		{"key and token", testAgentKey, token, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		})
	}
}
