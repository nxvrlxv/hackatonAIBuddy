package buddy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
)

// Интеграционные тесты хранилища чатов на настоящем Postgres.
// Нужен DATABASE_URL и применённые миграции до 0004_chat включительно.

type chatEnv struct {
	pool  *pgxpool.Pool
	repo  *Repository
	owner int64
	other int64
}

func newChatEnv(t *testing.T) *chatEnv {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(pool, ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var teamID int64
	if err := pool.QueryRow(ctx, `INSERT INTO buddy.teams (code, name) VALUES ($1, 'chat test') RETURNING id`,
		"chat-test-"+suffix).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	newUser := func(login string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO buddy.users (login, password_hash, full_name, role, team_id)
			VALUES ($1, '!', 'Chat Test', 'intern', $2) RETURNING id`, login+"-"+suffix, teamID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	env := &chatEnv{pool: pool, repo: repo, owner: newUser("chat-owner"), other: newUser("chat-other")}
	t.Cleanup(func() {
		// Чаты, ходы и сообщения удалятся каскадом.
		_, _ = pool.Exec(context.Background(), `DELETE FROM buddy.users WHERE id = ANY($1)`, []int64{env.owner, env.other})
		_, _ = pool.Exec(context.Background(), `DELETE FROM buddy.teams WHERE id = $1`, teamID)
	})
	return env
}

func (e *chatEnv) conversation(t *testing.T) int64 {
	t.Helper()
	c, err := e.repo.CreateConversation(context.Background(), e.owner, "test")
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func (e *chatEnv) turn(convID int64, clientID string) (chat.Turn, error) {
	return e.repo.StartTurn(context.Background(), chat.StartTurnInput{
		UserID: e.owner, ConversationID: convID, ClientMessageID: clientID,
		Payload: json.RawMessage(`{"content":"q"}`), HistoryLimit: 50,
	})
}

func (e *chatEnv) appendMsg(convID int64, runID, key string) (chat.Message, bool, error) {
	return e.repo.AppendMessage(context.Background(), chat.AppendInput{
		UserID: e.owner, ConversationID: convID, RunID: runID, EventKey: key,
		Role: chat.RoleAssistant, Payload: json.RawMessage(`{"content":"a","tool_calls":[]}`),
	})
}

func TestChatIntegrationFlow(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	conv := e.conversation(t)

	t1, err := e.turn(conv, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if t1.UserMessage.SequenceNumber != 1 || len(t1.History) != 0 {
		t.Fatalf("first turn: %+v", t1)
	}

	// Повтор с тем же event_key отдаёт ту же запись.
	a1, created, err := e.appendMsg(conv, t1.RunID, "a1")
	if err != nil || !created || a1.SequenceNumber != 2 {
		t.Fatalf("append: %+v %v %v", a1, created, err)
	}
	again, created, err := e.appendMsg(conv, t1.RunID, "a1")
	if err != nil || created || again.ID != a1.ID {
		t.Fatalf("replay: %+v %v %v", again, created, err)
	}

	// Двойной finish: первый статус сохраняется.
	run, err := e.repo.FinishRun(ctx, e.owner, conv, t1.RunID, chat.StatusCompleted)
	if err != nil || run.Status != chat.StatusCompleted || run.FinishedAt == nil {
		t.Fatalf("finish: %+v %v", run, err)
	}
	run, err = e.repo.FinishRun(ctx, e.owner, conv, t1.RunID, chat.StatusFailed)
	if err != nil || run.Status != chat.StatusCompleted {
		t.Fatalf("second finish must keep status: %+v %v", run, err)
	}

	// Повтор после finish всё ещё принимается, новая запись — нет.
	if again, created, err := e.appendMsg(conv, t1.RunID, "a1"); err != nil || created || again.ID != a1.ID {
		t.Fatalf("replay after finish: %+v %v %v", again, created, err)
	}
	if _, _, err := e.appendMsg(conv, t1.RunID, "a2"); !errors.Is(err, chat.ErrRunNotActive) {
		t.Fatalf("append after finish: %v", err)
	}

	// Повторная отправка того же client_message_id.
	if _, err := e.turn(conv, "m1"); !errors.Is(err, chat.ErrDuplicateMessage) {
		t.Fatalf("duplicate: %v", err)
	}

	// Второй ход видит историю первого.
	t2, err := e.turn(conv, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if len(t2.History) != 2 || t2.History[0].SequenceNumber != 1 || t2.History[1].EventKey != "a1" || t2.UserMessage.SequenceNumber != 3 {
		t.Fatalf("history: %+v", t2)
	}
	if string(t2.History[1].Payload) == "" {
		t.Fatal("payload must be returned")
	}

	msgs, err := e.repo.ListMessages(ctx, chat.ListMessagesInput{UserID: e.owner, ConversationID: conv, Limit: 2})
	if err != nil || len(msgs) != 2 || msgs[0].SequenceNumber != 2 || msgs[1].SequenceNumber != 3 {
		t.Fatalf("list messages: %+v %v", msgs, err)
	}
	msgs, err = e.repo.ListMessages(ctx, chat.ListMessagesInput{UserID: e.owner, ConversationID: conv, Limit: 10, Before: 2})
	if err != nil || len(msgs) != 1 || msgs[0].SequenceNumber != 1 {
		t.Fatalf("list before: %+v %v", msgs, err)
	}

	list, err := e.repo.ListConversations(ctx, e.owner)
	if err != nil || len(list) != 1 || list[0].LastSequence != 3 {
		t.Fatalf("list conversations: %+v %v", list, err)
	}
}

func TestChatIntegrationForeignUser(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	conv := e.conversation(t)
	t1, err := e.turn(conv, "m1")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.repo.StartTurn(ctx, chat.StartTurnInput{
		UserID: e.other, ConversationID: conv, ClientMessageID: "x", Payload: json.RawMessage(`{}`), HistoryLimit: 10,
	}); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("turn: %v", err)
	}
	if _, err := e.repo.ListMessages(ctx, chat.ListMessagesInput{UserID: e.other, ConversationID: conv, Limit: 10}); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("list: %v", err)
	}
	if _, _, err := e.repo.AppendMessage(ctx, chat.AppendInput{
		UserID: e.other, ConversationID: conv, RunID: t1.RunID, EventKey: "k", Role: chat.RoleTool, Payload: json.RawMessage(`{}`),
	}); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("append: %v", err)
	}
	if _, err := e.repo.FinishRun(ctx, e.other, conv, t1.RunID, chat.StatusFailed); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("finish: %v", err)
	}
	if list, err := e.repo.ListConversations(ctx, e.other); err != nil || len(list) != 0 {
		t.Fatalf("other's list: %+v %v", list, err)
	}
	// Чужой не смог завершить ход.
	if run, err := e.repo.FinishRun(ctx, e.owner, conv, t1.RunID, chat.StatusCompleted); err != nil || run.Status != chat.StatusCompleted {
		t.Fatalf("owner finish: %+v %v", run, err)
	}
}

func TestChatIntegrationConcurrentTurns(t *testing.T) {
	e := newChatEnv(t)
	conv := e.conversation(t)

	const n = 2
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = e.turn(conv, fmt.Sprintf("parallel-%d", i))
		}(i)
	}
	wg.Wait()

	ok, inProgress := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, chat.ErrRunInProgress):
			inProgress++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || inProgress != 1 {
		t.Fatalf("ok=%d in_progress=%d", ok, inProgress)
	}
}

func TestChatIntegrationStaleRun(t *testing.T) {
	e := newChatEnv(t)
	ctx := context.Background()
	conv := e.conversation(t)

	t1, err := e.turn(conv, "m1")
	if err != nil {
		t.Fatal(err)
	}
	// Делаем ход протухшим.
	if _, err := e.pool.Exec(ctx, `UPDATE buddy.runs SET last_activity_at = now() - interval '10 minutes' WHERE id = $1`, t1.RunID); err != nil {
		t.Fatal(err)
	}

	// Новый ход прерывает протухший.
	t2, err := e.turn(conv, "m2")
	if err != nil {
		t.Fatalf("turn after stale: %v", err)
	}
	if _, _, err := e.appendMsg(conv, t1.RunID, "late"); !errors.Is(err, chat.ErrRunNotActive) {
		t.Fatalf("append to interrupted: %v", err)
	}
	if run, err := e.repo.FinishRun(ctx, e.owner, conv, t1.RunID, chat.StatusCompleted); err != nil || run.Status != chat.StatusInterrupted {
		t.Fatalf("finish interrupted: %+v %v", run, err)
	}

	// Протухший running-ход без нового turns: append сам переводит его в interrupted.
	if _, err := e.pool.Exec(ctx, `UPDATE buddy.runs SET last_activity_at = now() - interval '10 minutes' WHERE id = $1`, t2.RunID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.appendMsg(conv, t2.RunID, "late"); !errors.Is(err, chat.ErrRunNotActive) {
		t.Fatalf("append to stale: %v", err)
	}
	var status string
	if err := e.pool.QueryRow(ctx, `SELECT status FROM buddy.runs WHERE id = $1`, t2.RunID).Scan(&status); err != nil || status != chat.StatusInterrupted {
		t.Fatalf("stale run status %q %v", status, err)
	}
}
