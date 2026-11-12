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

const agentTestRunID = "1b4e28ba-2fa1-11d2-883f-0016d3cca427"

// fakeChat отдаёт err, если он задан, и запоминает аргументы.
type fakeChat struct {
	err      error
	created  bool
	userID   int64
	listIn   chat.ListMessagesInput
	turnIn   chat.StartTurnInput
	appendIn chat.AppendInput
	finish   [2]string
}

func (f *fakeChat) CreateConversation(_ context.Context, userID int64, title string) (chat.Conversation, error) {
	f.userID = userID
	return chat.Conversation{ID: 3, Title: title}, f.err
}

func (f *fakeChat) ListConversations(_ context.Context, userID int64) ([]chat.Conversation, error) {
	f.userID = userID
	return []chat.Conversation{{ID: 3}, {ID: 4}}, f.err
}

func (f *fakeChat) ListMessages(_ context.Context, in chat.ListMessagesInput) ([]chat.Message, error) {
	f.listIn = in
	return []chat.Message{{ID: 1, Payload: json.RawMessage(`{"a":1}`)}}, f.err
}

func (f *fakeChat) StartTurn(_ context.Context, in chat.StartTurnInput) (chat.Turn, error) {
	f.turnIn = in
	return chat.Turn{
		RunID:       agentTestRunID,
		UserMessage: chat.Message{ID: 9, Role: "user", Payload: in.Payload},
		History:     []chat.Message{{ID: 8, Role: "assistant", Payload: json.RawMessage(`{}`)}},
	}, f.err
}

func (f *fakeChat) AppendMessage(_ context.Context, in chat.AppendInput) (chat.Message, bool, error) {
	f.appendIn = in
	return chat.Message{ID: 10, RunID: in.RunID, Payload: in.Payload}, f.created, f.err
}

func (f *fakeChat) FinishRun(_ context.Context, _, conversationID int64, runID, status string) (chat.Run, error) {
	f.finish = [2]string{runID, status}
	now := time.Now()
	return chat.Run{ID: runID, ConversationID: conversationID, Status: status, FinishedAt: &now}, f.err
}

// newAgentTestMux: claims подставляются через wrap, как это сделает requireAuth.
func newAgentTestMux(svc ChatService, withClaims bool) *http.ServeMux {
	mux := http.NewServeMux()
	NewAgentHandlers(svc).Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if withClaims {
				r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{UserID: 42}))
			}
			next.ServeHTTP(w, r)
		})
	})
	return mux
}

func doAgent(t *testing.T, mux http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

const (
	convPath   = "/internal/agent/conversations"
	turnBody   = `{"client_message_id":"m1","payload":{"content":"hi"}}`
	appendBody = `{"event_key":"e1","role":"assistant","payload":{"content":"ok"}}`
	finishBody = `{"status":"completed"}`
)

func runPath(suffix string) string {
	return convPath + "/7/runs/" + agentTestRunID + suffix
}

func TestAgentHandlersHappyPath(t *testing.T) {
	f := &fakeChat{}
	mux := newAgentTestMux(f, true)

	rec, out := doAgent(t, mux, "POST", convPath, `{"title":"Первый"}`)
	if rec.Code != http.StatusCreated || out["title"] != "Первый" || f.userID != 42 {
		t.Fatalf("create: %d %v", rec.Code, out)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	// Пустое тело тоже допустимо.
	if rec, _ := doAgent(t, mux, "POST", convPath, ""); rec.Code != http.StatusCreated {
		t.Fatalf("create empty: %d", rec.Code)
	}

	rec, out = doAgent(t, mux, "GET", convPath, "")
	if rec.Code != http.StatusOK || len(out["conversations"].([]any)) != 2 {
		t.Fatalf("list: %d %v", rec.Code, out)
	}

	rec, out = doAgent(t, mux, "GET", convPath+"/7/messages?limit=20&before=100", "")
	if rec.Code != http.StatusOK || len(out["messages"].([]any)) != 1 {
		t.Fatalf("messages: %d %v", rec.Code, out)
	}
	if f.listIn != (chat.ListMessagesInput{UserID: 42, ConversationID: 7, Limit: 20, Before: 100}) {
		t.Fatalf("list input %+v", f.listIn)
	}

	rec, out = doAgent(t, mux, "POST", convPath+"/7/turns", turnBody)
	if rec.Code != http.StatusCreated || out["run_id"] != agentTestRunID {
		t.Fatalf("turn: %d %v", rec.Code, out)
	}
	summary := out["summary"].(map[string]any)
	if summary["text"] != "" || summary["version"].(float64) != 0 || summary["through_sequence"].(float64) != 0 {
		t.Fatalf("summary %v", summary)
	}
	if len(out["history"].([]any)) != 1 || out["user_message"].(map[string]any)["payload"].(map[string]any)["content"] != "hi" {
		t.Fatalf("turn body %v", out)
	}
	if f.turnIn.ClientMessageID != "m1" || f.turnIn.UserID != 42 || f.turnIn.ConversationID != 7 {
		t.Fatalf("turn input %+v", f.turnIn)
	}

	f.created = true
	rec, out = doAgent(t, mux, "POST", runPath("/messages"), appendBody)
	if rec.Code != http.StatusCreated || out["run_id"] != agentTestRunID {
		t.Fatalf("append: %d %v", rec.Code, out)
	}
	if f.appendIn.RunID != agentTestRunID || f.appendIn.EventKey != "e1" || f.appendIn.Role != "assistant" {
		t.Fatalf("append input %+v", f.appendIn)
	}
	f.created = false
	if rec, _ := doAgent(t, mux, "POST", runPath("/messages"), appendBody); rec.Code != http.StatusOK {
		t.Fatalf("append replay: %d", rec.Code)
	}

	rec, out = doAgent(t, mux, "POST", runPath("/finish"), finishBody)
	if rec.Code != http.StatusOK || out["status"] != "completed" || out["run_id"] != agentTestRunID || out["finished_at"] == nil {
		t.Fatalf("finish: %d %v", rec.Code, out)
	}
	if f.finish != [2]string{agentTestRunID, "completed"} {
		t.Fatalf("finish args %v", f.finish)
	}
}

type agentRoute struct {
	method, path, body string
}

var allAgentRoutes = []agentRoute{
	{"POST", convPath, `{"title":"x"}`},
	{"GET", convPath, ""},
	{"GET", convPath + "/7/messages", ""},
	{"POST", convPath + "/7/turns", turnBody},
	{"POST", runPath("/messages"), appendBody},
	{"POST", runPath("/finish"), finishBody},
}

func TestAgentHandlersUnauthenticated(t *testing.T) {
	mux := newAgentTestMux(&fakeChat{}, false)
	for _, rt := range allAgentRoutes {
		rec, out := doAgent(t, mux, rt.method, rt.path, rt.body)
		if rec.Code != http.StatusUnauthorized || out["error"] != "unauthenticated" {
			t.Errorf("%s %s: %d %v", rt.method, rt.path, rec.Code, out)
		}
	}
}

func TestAgentHandlersServiceErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		msg    string
	}{
		{fmt.Errorf("conv: %w", chat.ErrNotFound), http.StatusNotFound, "not found"},
		{chat.ErrDuplicateMessage, http.StatusConflict, "duplicate message"},
		{chat.ErrRunInProgress, http.StatusConflict, "run in progress"},
		{chat.ErrRunNotActive, http.StatusConflict, "run not active"},
		{fmt.Errorf("%w: limit is bad", chat.ErrInvalidInput), http.StatusBadRequest, "limit is bad"},
		{errors.New("db exploded"), http.StatusInternalServerError, "internal error"},
	}
	for _, tc := range cases {
		mux := newAgentTestMux(&fakeChat{err: tc.err}, true)
		for _, rt := range allAgentRoutes {
			rec, out := doAgent(t, mux, rt.method, rt.path, rt.body)
			if rec.Code != tc.status || out["error"] != tc.msg {
				t.Errorf("%v → %s %s: %d %v", tc.err, rt.method, rt.path, rec.Code, out)
			}
		}
	}
}

func TestAgentHandlersBadRequests(t *testing.T) {
	mux := newAgentTestMux(&fakeChat{}, true)
	cases := []struct {
		method, path, body string
		status             int
		msg                string
	}{
		{"POST", convPath, `{bad`, 400, "invalid json"},
		{"GET", convPath + "/abc/messages", "", 400, "invalid conversation id"},
		{"GET", convPath + "/0/messages", "", 400, "invalid conversation id"},
		{"GET", convPath + "/7/messages?limit=x", "", 400, "invalid limit"},
		{"GET", convPath + "/7/messages?before=x", "", 400, "invalid before"},
		{"POST", convPath + "/abc/turns", turnBody, 400, "invalid conversation id"},
		{"POST", convPath + "/7/turns", "", 400, "invalid json"},
		{"POST", convPath + "/7/turns", `{"payload":` + strings.Repeat(" ", 1<<20) + `1}`, 413, "request body too large"},
		{"POST", convPath + "/x/runs/" + agentTestRunID + "/messages", appendBody, 400, "invalid conversation id"},
		{"POST", runPath("/messages"), `[`, 400, "invalid json"},
		{"POST", convPath + "/x/runs/" + agentTestRunID + "/finish", finishBody, 400, "invalid conversation id"},
		{"POST", runPath("/finish"), `nope`, 400, "invalid json"},
	}
	for _, tc := range cases {
		rec, out := doAgent(t, mux, tc.method, tc.path, tc.body)
		if rec.Code != tc.status || out["error"] != tc.msg {
			t.Errorf("%s %s: %d %v", tc.method, tc.path, rec.Code, out)
		}
	}
}

func TestAgentRegisterNilWrap(t *testing.T) {
	mux := http.NewServeMux()
	NewAgentHandlers(&fakeChat{}).Register(mux, nil)
	// Без wrap claims не появятся — ручка должна ответить 401, а не паниковать.
	rec, _ := doAgent(t, mux, "GET", convPath, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}
