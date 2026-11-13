package buddy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
	"github.com/pashagolub/pgxmock/v4"
)

const mockRunID = "1b4e28ba-2fa1-11d2-883f-0016d3cca427"

var (
	errDB       = errors.New("database failure")
	mockPayload = json.RawMessage(`{"content":"hi"}`)
	mockNow     = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	msgCols     = []string{"id", "conversation_id", "run_id", "sequence_number", "event_key", "role", "payload", "created_at"}
)

func newMockRepo(t *testing.T) (pgxmock.PgxPoolIface, *Repository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, NewRepositoryFromDB(db)
}

func checkMock(t *testing.T, db pgxmock.PgxPoolIface) {
	t.Helper()
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func msgRows() *pgxmock.Rows {
	return pgxmock.NewRows(msgCols)
}

func TestCreateConversation(t *testing.T) {
	db, repo := newMockRepo(t)
	q := `INSERT INTO buddy.conversations \(user_id, title\)`
	db.ExpectQuery(q).WithArgs(int64(1), "t").
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "last_sequence", "created_at", "updated_at"}).
			AddRow(int64(5), "t", int64(0), mockNow, mockNow))
	c, err := repo.CreateConversation(context.Background(), 1, "t")
	if err != nil || c.ID != 5 || c.Title != "t" {
		t.Fatalf("got %+v %v", c, err)
	}

	db.ExpectQuery(q).WithArgs(int64(2), "").WillReturnError(&pgconn.PgError{Code: pgForeignKeyViolation})
	if _, err := repo.CreateConversation(context.Background(), 2, ""); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("fk: got %v", err)
	}
	db.ExpectQuery(q).WithArgs(int64(3), "").WillReturnError(errDB)
	if _, err := repo.CreateConversation(context.Background(), 3, ""); !errors.Is(err, errDB) {
		t.Fatalf("got %v", err)
	}
	checkMock(t, db)
}

func TestListConversations(t *testing.T) {
	db, repo := newMockRepo(t)
	q := `FROM buddy.conversations\s+WHERE user_id = \$1`
	cols := []string{"id", "title", "last_sequence", "created_at", "updated_at"}

	db.ExpectQuery(q).WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows(cols).AddRow(int64(2), "b", int64(4), mockNow, mockNow).AddRow(int64(1), "a", int64(0), mockNow, mockNow))
	list, err := repo.ListConversations(context.Background(), 1)
	if err != nil || len(list) != 2 || list[0].ID != 2 {
		t.Fatalf("got %+v %v", list, err)
	}

	db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(cols))
	if list, err := repo.ListConversations(context.Background(), 1); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty: %+v %v", list, err)
	}

	db.ExpectQuery(q).WithArgs(int64(1)).WillReturnError(errDB)
	if _, err := repo.ListConversations(context.Background(), 1); !errors.Is(err, errDB) {
		t.Fatalf("query: %v", err)
	}

	db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(cols).AddRow("bad", "a", int64(0), mockNow, mockNow))
	if _, err := repo.ListConversations(context.Background(), 1); err == nil {
		t.Fatal("expected scan error")
	}

	db.ExpectQuery(q).WithArgs(int64(1)).WillReturnRows(pgxmock.NewRows(cols).AddRow(int64(1), "a", int64(0), mockNow, mockNow).AddRow(int64(2), "b", int64(0), mockNow, mockNow).CloseError(errDB))
	if _, err := repo.ListConversations(context.Background(), 1); !errors.Is(err, errDB) {
		t.Fatalf("rows err: %v", err)
	}
	checkMock(t, db)
}

func TestListMessages(t *testing.T) {
	db, repo := newMockRepo(t)
	in := chat.ListMessagesInput{UserID: 1, ConversationID: 2, Limit: 10, Before: 50}
	exists := `SELECT EXISTS \(SELECT 1 FROM buddy.conversations WHERE id = \$1 AND user_id = \$2\)`
	list := `FROM buddy.messages\s+WHERE conversation_id = \$1 AND`

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(true))
	db.ExpectQuery(list).WithArgs(int64(2), int64(50), 10).
		WillReturnRows(msgRows().AddRow(int64(1), int64(2), mockRunID, int64(1), "user", "user", mockPayload, mockNow))
	msgs, err := repo.ListMessages(context.Background(), in)
	if err != nil || len(msgs) != 1 || string(msgs[0].Payload) != string(mockPayload) || msgs[0].RunID != mockRunID {
		t.Fatalf("got %+v %v", msgs, err)
	}

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(false))
	if _, err := repo.ListMessages(context.Background(), in); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("foreign: %v", err)
	}

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnError(errDB)
	if _, err := repo.ListMessages(context.Background(), in); !errors.Is(err, errDB) {
		t.Fatalf("exists err: %v", err)
	}

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(true))
	db.ExpectQuery(list).WithArgs(int64(2), int64(50), 10).WillReturnError(errDB)
	if _, err := repo.ListMessages(context.Background(), in); !errors.Is(err, errDB) {
		t.Fatalf("list err: %v", err)
	}

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(true))
	db.ExpectQuery(list).WithArgs(int64(2), int64(50), 10).
		WillReturnRows(msgRows().AddRow("bad", int64(2), mockRunID, int64(1), "user", "user", mockPayload, mockNow))
	if _, err := repo.ListMessages(context.Background(), in); err == nil {
		t.Fatal("expected scan error")
	}

	db.ExpectQuery(exists).WithArgs(int64(2), int64(1)).WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(true))
	db.ExpectQuery(list).WithArgs(int64(2), int64(50), 10).
		WillReturnRows(msgRows().AddRow(int64(1), int64(2), mockRunID, int64(1), "user", "user", mockPayload, mockNow).AddRow(int64(2), int64(2), mockRunID, int64(2), "a", "assistant", mockPayload, mockNow).CloseError(errDB))
	if _, err := repo.ListMessages(context.Background(), in); !errors.Is(err, errDB) {
		t.Fatalf("rows err: %v", err)
	}
	checkMock(t, db)
}

const (
	qLock      = `SELECT id FROM buddy.conversations\s+WHERE id = \$1 AND user_id = \$2\s+FOR UPDATE`
	qStale     = `UPDATE buddy.runs SET status = 'interrupted', finished_at = now\(\)\s+WHERE conversation_id = \$1`
	qDupCheck  = `SELECT EXISTS \(SELECT 1 FROM buddy.runs WHERE conversation_id = \$1 AND client_message_id = \$2\)`
	qInsertRun = `INSERT INTO buddy.runs \(conversation_id, client_message_id\)`
	qNextSeq   = `UPDATE buddy.conversations\s+SET last_sequence = last_sequence \+ 1`
	qInsertMsg = `INSERT INTO buddy.messages`
	qHistory   = `WHERE conversation_id = \$1 AND sequence_number < \$2`
)

// expectStartTurn строит цепочку ожиданий StartTurn; failAt — шаг, на котором база отвечает ошибкой.
func expectStartTurn(db pgxmock.PgxPoolIface, failAt string, failErr error) {
	if failAt == "begin" {
		db.ExpectBegin().WillReturnError(failErr)
		return
	}
	db.ExpectBegin()
	defer db.ExpectRollback()

	lock := db.ExpectQuery(qLock).WithArgs(int64(2), int64(1))
	switch failAt {
	case "lock-missing":
		lock.WillReturnError(pgx.ErrNoRows)
		return
	case "lock":
		lock.WillReturnError(failErr)
		return
	}
	lock.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(2)))

	stale := db.ExpectExec(qStale).WithArgs(int64(2))
	if failAt == "stale" {
		stale.WillReturnError(failErr)
		return
	}
	stale.WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	dup := db.ExpectQuery(qDupCheck).WithArgs(int64(2), "m1")
	switch failAt {
	case "dupcheck":
		dup.WillReturnError(failErr)
		return
	case "duplicate":
		dup.WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(true))
		return
	}
	dup.WillReturnRows(pgxmock.NewRows([]string{"e"}).AddRow(false))

	ins := db.ExpectQuery(qInsertRun).WithArgs(int64(2), "m1")
	if failAt == "insert-run" {
		ins.WillReturnError(failErr)
		return
	}
	ins.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(mockRunID))

	seq := db.ExpectQuery(qNextSeq).WithArgs(int64(2))
	if failAt == "seq" {
		seq.WillReturnError(failErr)
		return
	}
	seq.WillReturnRows(pgxmock.NewRows([]string{"last_sequence"}).AddRow(int64(3)))

	msg := db.ExpectQuery(qInsertMsg).WithArgs(int64(2), mockRunID, int64(3), chat.UserEventKey, chat.RoleUser, mockPayload)
	if failAt == "insert-msg" {
		msg.WillReturnError(failErr)
		return
	}
	msg.WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(30), mockNow))

	hist := db.ExpectQuery(qHistory).WithArgs(int64(2), int64(3), 5)
	switch failAt {
	case "history":
		hist.WillReturnError(failErr)
		return
	case "history-scan":
		hist.WillReturnRows(msgRows().AddRow("bad", int64(2), mockRunID, int64(1), "user", "user", mockPayload, mockNow))
		return
	}
	hist.WillReturnRows(msgRows().
		AddRow(int64(10), int64(2), mockRunID, int64(1), "user", "user", mockPayload, mockNow).
		AddRow(int64(11), int64(2), mockRunID, int64(2), "a1", "assistant", json.RawMessage(`{"content":"yo"}`), mockNow))

	commit := db.ExpectCommit()
	if failAt == "commit" {
		commit.WillReturnError(failErr)
	}
}

func TestStartTurn(t *testing.T) {
	in := chat.StartTurnInput{UserID: 1, ConversationID: 2, ClientMessageID: "m1", Payload: mockPayload, HistoryLimit: 5}
	cases := []struct {
		failAt  string
		failErr error
		want    error
	}{
		{"begin", errDB, errDB},
		{"lock-missing", nil, chat.ErrNotFound},
		{"lock", errDB, errDB},
		{"stale", errDB, errDB},
		{"dupcheck", errDB, errDB},
		{"duplicate", nil, chat.ErrDuplicateMessage},
		{"insert-run", &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: constraintOneRunning}, chat.ErrRunInProgress},
		{"insert-run", &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "runs_conversation_id_client_message_id_key"}, chat.ErrDuplicateMessage},
		{"insert-run", errDB, errDB},
		{"seq", errDB, errDB},
		{"insert-msg", errDB, errDB},
		{"history", errDB, errDB},
		{"history-scan", nil, nil},
		{"commit", errDB, errDB},
	}
	for _, tc := range cases {
		t.Run(tc.failAt, func(t *testing.T) {
			db, repo := newMockRepo(t)
			expectStartTurn(db, tc.failAt, tc.failErr)
			turn, err := repo.StartTurn(context.Background(), in)
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if err == nil {
				t.Fatalf("expected error, got %+v", turn)
			}
			checkMock(t, db)
		})
	}

	t.Run("ok", func(t *testing.T) {
		db, repo := newMockRepo(t)
		expectStartTurn(db, "", nil)
		turn, err := repo.StartTurn(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if turn.RunID != mockRunID || turn.UserMessage.ID != 30 || turn.UserMessage.SequenceNumber != 3 ||
			turn.UserMessage.Role != chat.RoleUser || len(turn.History) != 2 || turn.History[1].EventKey != "a1" {
			t.Fatalf("got %+v", turn)
		}
		checkMock(t, db)
	})
}

const (
	qReplay  = `FROM buddy.messages\s+WHERE conversation_id = \$1 AND run_id = \$2::uuid AND event_key = \$3`
	qRunLock = `SELECT status, last_activity_at < now\(\) - interval '5 minutes'\s+FROM buddy.runs`
	qKill    = `UPDATE buddy.runs SET status = 'interrupted', finished_at = now\(\)\s+WHERE id = \$1::uuid`
	qTouch   = `UPDATE buddy.runs SET last_activity_at = now\(\) WHERE id = \$1::uuid`
)

func expectAppend(db pgxmock.PgxPoolIface, failAt string) {
	if failAt == "begin" {
		db.ExpectBegin().WillReturnError(errDB)
		return
	}
	db.ExpectBegin()
	defer db.ExpectRollback()

	lock := db.ExpectQuery(qLock).WithArgs(int64(2), int64(1))
	switch failAt {
	case "lock-missing":
		lock.WillReturnError(pgx.ErrNoRows)
		return
	}
	lock.WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(int64(2)))

	replay := db.ExpectQuery(qReplay).WithArgs(int64(2), mockRunID, "e1")
	switch failAt {
	case "replay":
		replay.WillReturnRows(msgRows().AddRow(int64(77), int64(2), mockRunID, int64(4), "e1", "assistant", mockPayload, mockNow))
		return
	case "replay-err":
		replay.WillReturnError(errDB)
		return
	}
	replay.WillReturnError(pgx.ErrNoRows)

	run := db.ExpectQuery(qRunLock).WithArgs(mockRunID, int64(2))
	switch failAt {
	case "run-missing":
		run.WillReturnError(pgx.ErrNoRows)
		return
	case "run-err":
		run.WillReturnError(errDB)
		return
	case "run-finished":
		run.WillReturnRows(pgxmock.NewRows([]string{"status", "stale"}).AddRow(chat.StatusCompleted, false))
		return
	case "run-stale", "run-stale-kill-err", "run-stale-commit-err":
		run.WillReturnRows(pgxmock.NewRows([]string{"status", "stale"}).AddRow(chat.StatusRunning, true))
		kill := db.ExpectExec(qKill).WithArgs(mockRunID)
		if failAt == "run-stale-kill-err" {
			kill.WillReturnError(errDB)
			return
		}
		kill.WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		commit := db.ExpectCommit()
		if failAt == "run-stale-commit-err" {
			commit.WillReturnError(errDB)
		}
		return
	}
	run.WillReturnRows(pgxmock.NewRows([]string{"status", "stale"}).AddRow(chat.StatusRunning, false))

	seq := db.ExpectQuery(qNextSeq).WithArgs(int64(2))
	if failAt == "seq" {
		seq.WillReturnError(errDB)
		return
	}
	seq.WillReturnRows(pgxmock.NewRows([]string{"last_sequence"}).AddRow(int64(5)))

	msg := db.ExpectQuery(qInsertMsg).WithArgs(int64(2), mockRunID, int64(5), "e1", chat.RoleAssistant, mockPayload)
	if failAt == "insert" {
		msg.WillReturnError(errDB)
		return
	}
	msg.WillReturnRows(pgxmock.NewRows([]string{"id", "created_at"}).AddRow(int64(50), mockNow))

	touch := db.ExpectExec(qTouch).WithArgs(mockRunID)
	if failAt == "touch" {
		touch.WillReturnError(errDB)
		return
	}
	touch.WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	commit := db.ExpectCommit()
	if failAt == "commit" {
		commit.WillReturnError(errDB)
	}
}

func TestAppendMessage(t *testing.T) {
	in := chat.AppendInput{UserID: 1, ConversationID: 2, RunID: mockRunID, EventKey: "e1", Role: chat.RoleAssistant, Payload: mockPayload}
	cases := map[string]error{
		"begin":                errDB,
		"lock-missing":         chat.ErrNotFound,
		"replay-err":           errDB,
		"run-missing":          chat.ErrNotFound,
		"run-err":              errDB,
		"run-finished":         chat.ErrRunNotActive,
		"run-stale":            chat.ErrRunNotActive,
		"run-stale-kill-err":   errDB,
		"run-stale-commit-err": errDB,
		"seq":                  errDB,
		"insert":               errDB,
		"touch":                errDB,
		"commit":               errDB,
	}
	for failAt, want := range cases {
		t.Run(failAt, func(t *testing.T) {
			db, repo := newMockRepo(t)
			expectAppend(db, failAt)
			if _, _, err := repo.AppendMessage(context.Background(), in); !errors.Is(err, want) {
				t.Fatalf("want %v, got %v", want, err)
			}
			checkMock(t, db)
		})
	}

	t.Run("replay", func(t *testing.T) {
		db, repo := newMockRepo(t)
		expectAppend(db, "replay")
		msg, created, err := repo.AppendMessage(context.Background(), in)
		if err != nil || created || msg.ID != 77 {
			t.Fatalf("got %+v %v %v", msg, created, err)
		}
		checkMock(t, db)
	})

	t.Run("ok", func(t *testing.T) {
		db, repo := newMockRepo(t)
		expectAppend(db, "")
		msg, created, err := repo.AppendMessage(context.Background(), in)
		if err != nil || !created || msg.ID != 50 || msg.SequenceNumber != 5 || msg.EventKey != "e1" {
			t.Fatalf("got %+v %v %v", msg, created, err)
		}
		checkMock(t, db)
	})
}

func TestFinishRun(t *testing.T) {
	qUpdate := `UPDATE buddy.runs r SET status = \$4, finished_at = now\(\)`
	qGet := `SELECT r.status, r.finished_at\s+FROM buddy.runs r`
	args := []any{mockRunID, int64(2), int64(1)}
	cols := []string{"status", "finished_at"}
	finished := mockNow

	t.Run("changed", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(qUpdate).WithArgs(append(args, chat.StatusCompleted)...).
			WillReturnRows(pgxmock.NewRows(cols).AddRow(chat.StatusCompleted, &finished))
		run, err := repo.FinishRun(context.Background(), 1, 2, mockRunID, chat.StatusCompleted)
		if err != nil || run.Status != chat.StatusCompleted || run.FinishedAt == nil || run.ID != mockRunID || run.ConversationID != 2 {
			t.Fatalf("got %+v %v", run, err)
		}
		checkMock(t, db)
	})

	t.Run("already finished", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(qUpdate).WithArgs(append(args, chat.StatusFailed)...).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(qGet).WithArgs(args...).WillReturnRows(pgxmock.NewRows(cols).AddRow(chat.StatusCompleted, &finished))
		run, err := repo.FinishRun(context.Background(), 1, 2, mockRunID, chat.StatusFailed)
		if err != nil || run.Status != chat.StatusCompleted {
			t.Fatalf("got %+v %v", run, err)
		}
		checkMock(t, db)
	})

	t.Run("not found", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(qUpdate).WithArgs(append(args, chat.StatusFailed)...).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(qGet).WithArgs(args...).WillReturnError(pgx.ErrNoRows)
		if _, err := repo.FinishRun(context.Background(), 1, 2, mockRunID, chat.StatusFailed); !errors.Is(err, chat.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
		checkMock(t, db)
	})

	t.Run("update error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(qUpdate).WithArgs(append(args, chat.StatusFailed)...).WillReturnError(errDB)
		if _, err := repo.FinishRun(context.Background(), 1, 2, mockRunID, chat.StatusFailed); !errors.Is(err, errDB) {
			t.Fatalf("got %v", err)
		}
		checkMock(t, db)
	})

	t.Run("get error", func(t *testing.T) {
		db, repo := newMockRepo(t)
		db.ExpectQuery(qUpdate).WithArgs(append(args, chat.StatusFailed)...).WillReturnError(pgx.ErrNoRows)
		db.ExpectQuery(qGet).WithArgs(args...).WillReturnError(errDB)
		if _, err := repo.FinishRun(context.Background(), 1, 2, mockRunID, chat.StatusFailed); !errors.Is(err, errDB) {
			t.Fatalf("got %v", err)
		}
		checkMock(t, db)
	})
}
