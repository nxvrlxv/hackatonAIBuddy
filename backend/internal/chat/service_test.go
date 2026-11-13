package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testRunID = "1b4e28ba-2fa1-11d2-883f-0016d3cca427"

// fakeStore запоминает последний вызов и отдаёт заданную ошибку.
type fakeStore struct {
	err       error
	title     string
	listIn    ListMessagesInput
	turnIn    StartTurnInput
	turn      Turn
	appendIn  AppendInput
	finishArg string
	calls     int
}

func (f *fakeStore) CreateConversation(_ context.Context, userID int64, title string) (Conversation, error) {
	f.calls++
	f.title = title
	return Conversation{ID: 1, Title: title}, f.err
}

func (f *fakeStore) ListConversations(context.Context, int64) ([]Conversation, error) {
	f.calls++
	return []Conversation{{ID: 1}}, f.err
}

func (f *fakeStore) ListMessages(_ context.Context, in ListMessagesInput) ([]Message, error) {
	f.calls++
	f.listIn = in
	return nil, f.err
}

func (f *fakeStore) StartTurn(_ context.Context, in StartTurnInput) (Turn, error) {
	f.calls++
	f.turnIn = in
	return f.turn, f.err
}

func (f *fakeStore) AppendMessage(_ context.Context, in AppendInput) (Message, bool, error) {
	f.calls++
	f.appendIn = in
	return Message{ID: 5}, true, f.err
}

func (f *fakeStore) FinishRun(_ context.Context, _, _ int64, _ string, status string) (Run, error) {
	f.calls++
	f.finishArg = status
	return Run{Status: status}, f.err
}

var payload = json.RawMessage(`{"content":"hi"}`)

func TestNewServiceDefaultHistory(t *testing.T) {
	if s := NewService(&fakeStore{}, 0); s.historyLimit != DefaultHistoryLimit {
		t.Fatalf("history limit = %d", s.historyLimit)
	}
	if s := NewService(&fakeStore{}, 7); s.historyLimit != 7 {
		t.Fatalf("history limit = %d", s.historyLimit)
	}
}

func TestCreateConversation(t *testing.T) {
	st := &fakeStore{}
	s := NewService(st, 0)
	c, err := s.CreateConversation(context.Background(), 1, "  hello  ")
	if err != nil || c.Title != "hello" || st.title != "hello" {
		t.Fatalf("got %+v %v", c, err)
	}
	_, err = s.CreateConversation(context.Background(), 1, strings.Repeat("я", MaxTitleLength+1))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if st.calls != 1 {
		t.Fatalf("store called on invalid input")
	}
}

func TestListConversations(t *testing.T) {
	st := &fakeStore{err: ErrNotFound}
	_, err := NewService(st, 0).ListConversations(context.Background(), 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestListMessages(t *testing.T) {
	st := &fakeStore{}
	s := NewService(st, 0)
	if _, err := s.ListMessages(context.Background(), ListMessagesInput{UserID: 1, ConversationID: 2}); err != nil {
		t.Fatal(err)
	}
	if st.listIn.Limit != DefaultMessageLimit {
		t.Fatalf("default limit not applied: %d", st.listIn.Limit)
	}
	for _, in := range []ListMessagesInput{{Limit: -1}, {Limit: MaxMessageLimit + 1}, {Limit: 10, Before: -1}} {
		if _, err := s.ListMessages(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%+v: expected invalid input, got %v", in, err)
		}
	}
	if st.calls != 1 {
		t.Fatalf("store called on invalid input: %d", st.calls)
	}
}

func TestStartTurn(t *testing.T) {
	st := &fakeStore{turn: Turn{RunID: testRunID, Summary: Summary{Text: "x", Version: 3}}}
	s := NewService(st, 10)
	turn, err := s.StartTurn(context.Background(), StartTurnInput{
		UserID: 1, ConversationID: 2, ClientMessageID: " m1 ", Payload: payload, HistoryLimit: 999,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.turnIn.HistoryLimit != 10 || st.turnIn.ClientMessageID != "m1" {
		t.Fatalf("bad store input: %+v", st.turnIn)
	}
	if turn.Summary != (Summary{}) || turn.History == nil {
		t.Fatalf("summary must be empty and history non-nil: %+v", turn)
	}

	st.err = ErrRunInProgress
	if _, err := s.StartTurn(context.Background(), StartTurnInput{ClientMessageID: "m2", Payload: payload}); !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("got %v", err)
	}
}

func TestStartTurnValidation(t *testing.T) {
	st := &fakeStore{}
	s := NewService(st, 0)
	cases := []StartTurnInput{
		{ClientMessageID: "", Payload: payload},
		{ClientMessageID: "   ", Payload: payload},
		{ClientMessageID: strings.Repeat("a", MaxKeyLength+1), Payload: payload},
		{ClientMessageID: "m", Payload: nil},
		{ClientMessageID: "m", Payload: json.RawMessage(" null ")},
		{ClientMessageID: "m", Payload: json.RawMessage(`{bad`)},
	}
	for _, in := range cases {
		if _, err := s.StartTurn(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%+v: expected invalid input, got %v", in, err)
		}
	}
	if st.calls != 0 {
		t.Fatalf("store called on invalid input")
	}
}

func TestAppendMessage(t *testing.T) {
	st := &fakeStore{}
	s := NewService(st, 0)
	msg, created, err := s.AppendMessage(context.Background(), AppendInput{
		RunID: testRunID, EventKey: " step-1 ", Role: RoleAssistant, Payload: payload,
	})
	if err != nil || !created || msg.ID != 5 || st.appendIn.EventKey != "step-1" {
		t.Fatalf("got %+v %v %v / %+v", msg, created, err, st.appendIn)
	}

	cases := []AppendInput{
		{RunID: "not-a-uuid", EventKey: "k", Role: RoleTool, Payload: payload},
		{RunID: testRunID, EventKey: " ", Role: RoleTool, Payload: payload},
		{RunID: testRunID, EventKey: strings.Repeat("k", MaxKeyLength+1), Role: RoleTool, Payload: payload},
		{RunID: testRunID, EventKey: "k", Role: "robot", Payload: payload},
		{RunID: testRunID, EventKey: "k", Role: RoleSystem, Payload: json.RawMessage("[")},
	}
	for _, in := range cases {
		if _, _, err := s.AppendMessage(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%+v: expected invalid input, got %v", in, err)
		}
	}

	st.err = ErrRunNotActive
	if _, _, err := s.AppendMessage(context.Background(), AppendInput{RunID: testRunID, EventKey: "k", Role: RoleUser, Payload: payload}); !errors.Is(err, ErrRunNotActive) {
		t.Fatalf("got %v", err)
	}
}

func TestFinishRun(t *testing.T) {
	st := &fakeStore{}
	s := NewService(st, 0)
	for _, status := range []string{StatusCompleted, StatusFailed, StatusInterrupted} {
		run, err := s.FinishRun(context.Background(), 1, 2, testRunID, status)
		if err != nil || run.Status != status {
			t.Fatalf("%s: %+v %v", status, run, err)
		}
	}
	if _, err := s.FinishRun(context.Background(), 1, 2, testRunID, StatusRunning); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("running must be rejected, got %v", err)
	}
	if _, err := s.FinishRun(context.Background(), 1, 2, "x", StatusCompleted); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad uuid must be rejected, got %v", err)
	}
	st.err = ErrNotFound
	if _, err := s.FinishRun(context.Background(), 1, 2, testRunID, StatusCompleted); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestIsUUID(t *testing.T) {
	good := []string{testRunID, "1B4E28BA-2FA1-11D2-883F-0016D3CCA427"}
	bad := []string{"", "1b4e28ba2fa111d2883f0016d3cca427", "1b4e28ba-2fa1-11d2-883f-0016d3cca42g", "1b4e28ba-2fa1-11d2-883f_0016d3cca427"}
	for _, s := range good {
		if !IsUUID(s) {
			t.Errorf("%q should be uuid", s)
		}
	}
	for _, s := range bad {
		if IsUUID(s) {
			t.Errorf("%q should not be uuid", s)
		}
	}
}
