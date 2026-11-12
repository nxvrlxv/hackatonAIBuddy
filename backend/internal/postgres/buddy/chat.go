package buddy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
)

// Хранилище чатов агента (реализует chat.Store).
//
// Порядок блокировок везде один: сначала строка conversations (FOR UPDATE), потом runs.
// Иначе StartTurn (conversation → runs) и AppendMessage могли бы взаимно заблокироваться.

const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	constraintOneRunning  = "runs_one_running"
)

var _ chat.Store = (*Repository)(nil)

const messageColumns = `id, conversation_id, run_id::text, sequence_number, event_key, role, payload, created_at`

func scanMessage(row pgx.Row) (chat.Message, error) {
	var m chat.Message
	err := row.Scan(&m.ID, &m.ConversationID, &m.RunID, &m.SequenceNumber, &m.EventKey, &m.Role, &m.Payload, &m.CreatedAt)
	return m, err
}

func collectMessages(rows pgx.Rows) ([]chat.Message, error) {
	defer rows.Close()
	result := []chat.Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read messages: %w", err)
	}
	return result, nil
}

func pgErrCode(err error) (code, constraint string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName
	}
	return "", ""
}

func (r *Repository) CreateConversation(ctx context.Context, userID int64, title string) (chat.Conversation, error) {
	var c chat.Conversation
	err := r.pool.QueryRow(ctx, `
		INSERT INTO buddy.conversations (user_id, title) VALUES ($1, $2)
		RETURNING id, title, last_sequence, created_at, updated_at`, userID, title).
		Scan(&c.ID, &c.Title, &c.LastSequence, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if code, _ := pgErrCode(err); code == pgForeignKeyViolation {
			return chat.Conversation{}, fmt.Errorf("user %d: %w", userID, chat.ErrNotFound)
		}
		return chat.Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return c, nil
}

func (r *Repository) ListConversations(ctx context.Context, userID int64) ([]chat.Conversation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, last_sequence, created_at, updated_at
		FROM buddy.conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	result := []chat.Conversation{}
	for rows.Next() {
		var c chat.Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.LastSequence, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read conversations: %w", err)
	}
	return result, nil
}

// ListMessages отдаёт последние Limit сообщений (с sequence_number < Before, если задан)
// по возрастанию sequence_number, с полным payload.
func (r *Repository) ListMessages(ctx context.Context, in chat.ListMessagesInput) ([]chat.Message, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM buddy.conversations WHERE id = $1 AND user_id = $2)`,
		in.ConversationID, in.UserID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check conversation: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("conversation %d: %w", in.ConversationID, chat.ErrNotFound)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT `+messageColumns+` FROM (
			SELECT * FROM buddy.messages
			WHERE conversation_id = $1 AND ($2::bigint = 0 OR sequence_number < $2::bigint)
			ORDER BY sequence_number DESC
			LIMIT $3
		) m
		ORDER BY sequence_number`, in.ConversationID, in.Before, in.Limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return collectMessages(rows)
}

// lockConversation блокирует строку чата пользователя до конца транзакции.
func lockConversation(ctx context.Context, tx pgx.Tx, userID, conversationID int64) error {
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT id FROM buddy.conversations
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, conversationID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conversation %d: %w", conversationID, chat.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("lock conversation: %w", err)
	}
	return nil
}

// nextSequence выдаёт следующий sequence_number чата (строка уже заблокирована).
func nextSequence(ctx context.Context, tx pgx.Tx, conversationID int64) (int64, error) {
	var seq int64
	err := tx.QueryRow(ctx, `
		UPDATE buddy.conversations
		SET last_sequence = last_sequence + 1, updated_at = now()
		WHERE id = $1
		RETURNING last_sequence`, conversationID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("next sequence: %w", err)
	}
	return seq, nil
}

func insertMessage(ctx context.Context, tx pgx.Tx, m chat.Message) (chat.Message, error) {
	err := tx.QueryRow(ctx, `
		INSERT INTO buddy.messages (conversation_id, run_id, sequence_number, event_key, role, payload)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
		RETURNING id, created_at`,
		m.ConversationID, m.RunID, m.SequenceNumber, m.EventKey, m.Role, m.Payload).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return chat.Message{}, fmt.Errorf("insert message: %w", err)
	}
	return m, nil
}

func (r *Repository) StartTurn(ctx context.Context, in chat.StartTurnInput) (chat.Turn, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return chat.Turn{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Чат пользователя под блокировкой: параллельные ходы выстраиваются в очередь.
	if err := lockConversation(ctx, tx, in.UserID, in.ConversationID); err != nil {
		return chat.Turn{}, err
	}

	// 2. Протухшие running-ходы помечаем прерванными.
	_, err = tx.Exec(ctx, `
		UPDATE buddy.runs SET status = 'interrupted', finished_at = now()
		WHERE conversation_id = $1 AND status = 'running'
		  AND last_activity_at < now() - interval '5 minutes'`, in.ConversationID)
	if err != nil {
		return chat.Turn{}, fmt.Errorf("interrupt stale runs: %w", err)
	}

	// 3. Новый ход. Дубликат проверяем явно, чтобы он не маскировался под runs_one_running.
	var duplicate bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM buddy.runs WHERE conversation_id = $1 AND client_message_id = $2)`,
		in.ConversationID, in.ClientMessageID).Scan(&duplicate)
	if err != nil {
		return chat.Turn{}, fmt.Errorf("check duplicate: %w", err)
	}
	if duplicate {
		return chat.Turn{}, chat.ErrDuplicateMessage
	}

	var runID string
	err = tx.QueryRow(ctx, `
		INSERT INTO buddy.runs (conversation_id, client_message_id) VALUES ($1, $2)
		RETURNING id::text`, in.ConversationID, in.ClientMessageID).Scan(&runID)
	if err != nil {
		code, constraint := pgErrCode(err)
		switch {
		case code == pgUniqueViolation && constraint == constraintOneRunning:
			return chat.Turn{}, chat.ErrRunInProgress
		case code == pgUniqueViolation:
			return chat.Turn{}, chat.ErrDuplicateMessage
		}
		return chat.Turn{}, fmt.Errorf("insert run: %w", err)
	}

	// 4. Сообщение пользователя.
	seq, err := nextSequence(ctx, tx, in.ConversationID)
	if err != nil {
		return chat.Turn{}, err
	}
	userMsg, err := insertMessage(ctx, tx, chat.Message{
		ConversationID: in.ConversationID,
		RunID:          runID,
		SequenceNumber: seq,
		EventKey:       chat.UserEventKey,
		Role:           chat.RoleUser,
		Payload:        in.Payload,
	})
	if err != nil {
		return chat.Turn{}, err
	}

	// 5. История до нового сообщения.
	rows, err := tx.Query(ctx, `
		SELECT `+messageColumns+` FROM (
			SELECT * FROM buddy.messages
			WHERE conversation_id = $1 AND sequence_number < $2
			  AND run_id IN (SELECT id FROM buddy.runs
			                 WHERE conversation_id = $1 AND status = 'completed')
			ORDER BY sequence_number DESC
			LIMIT $3
		) m
		ORDER BY sequence_number`, in.ConversationID, seq, in.HistoryLimit)
	if err != nil {
		return chat.Turn{}, fmt.Errorf("load history: %w", err)
	}
	history, err := collectMessages(rows)
	if err != nil {
		return chat.Turn{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return chat.Turn{}, fmt.Errorf("commit: %w", err)
	}
	return chat.Turn{RunID: runID, UserMessage: userMsg, History: history}, nil
}

func (r *Repository) AppendMessage(ctx context.Context, in chat.AppendInput) (chat.Message, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return chat.Message{}, false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockConversation(ctx, tx, in.UserID, in.ConversationID); err != nil {
		return chat.Message{}, false, err
	}

	// 1. Повтор с тем же event_key — отдаём существующую запись, даже если ход завершён.
	existing, err := scanMessage(tx.QueryRow(ctx, `
		SELECT `+messageColumns+` FROM buddy.messages
		WHERE conversation_id = $1 AND run_id = $2::uuid AND event_key = $3`,
		in.ConversationID, in.RunID, in.EventKey))
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return chat.Message{}, false, fmt.Errorf("find replay: %w", err)
	}

	// 2. Ход должен быть running и не протухшим.
	var status string
	var stale bool
	err = tx.QueryRow(ctx, `
		SELECT status, last_activity_at < now() - interval '5 minutes'
		FROM buddy.runs
		WHERE id = $1::uuid AND conversation_id = $2
		FOR UPDATE`, in.RunID, in.ConversationID).Scan(&status, &stale)
	if errors.Is(err, pgx.ErrNoRows) {
		return chat.Message{}, false, fmt.Errorf("run %s: %w", in.RunID, chat.ErrNotFound)
	}
	if err != nil {
		return chat.Message{}, false, fmt.Errorf("lock run: %w", err)
	}
	if status != chat.StatusRunning {
		return chat.Message{}, false, chat.ErrRunNotActive
	}
	if stale {
		// Фиксируем прерывание, чтобы статус в базе совпадал с ответом.
		if _, err := tx.Exec(ctx, `
			UPDATE buddy.runs SET status = 'interrupted', finished_at = now()
			WHERE id = $1::uuid`, in.RunID); err != nil {
			return chat.Message{}, false, fmt.Errorf("interrupt stale run: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return chat.Message{}, false, fmt.Errorf("commit: %w", err)
		}
		return chat.Message{}, false, chat.ErrRunNotActive
	}

	// 3–4. Номер, вставка, продление хода.
	seq, err := nextSequence(ctx, tx, in.ConversationID)
	if err != nil {
		return chat.Message{}, false, err
	}
	msg, err := insertMessage(ctx, tx, chat.Message{
		ConversationID: in.ConversationID,
		RunID:          in.RunID,
		SequenceNumber: seq,
		EventKey:       in.EventKey,
		Role:           in.Role,
		Payload:        in.Payload,
	})
	if err != nil {
		return chat.Message{}, false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE buddy.runs SET last_activity_at = now() WHERE id = $1::uuid`, in.RunID); err != nil {
		return chat.Message{}, false, fmt.Errorf("touch run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return chat.Message{}, false, fmt.Errorf("commit: %w", err)
	}
	return msg, true, nil
}

// FinishRun переводит running-ход в финальный статус. Если ход уже завершён,
// ничего не меняет и возвращает текущий статус.
func (r *Repository) FinishRun(ctx context.Context, userID, conversationID int64, runID, status string) (chat.Run, error) {
	run := chat.Run{ID: runID, ConversationID: conversationID}
	err := r.pool.QueryRow(ctx, `
		UPDATE buddy.runs r SET status = $4, finished_at = now()
		FROM buddy.conversations c
		WHERE r.id = $1::uuid AND r.conversation_id = $2
		  AND c.id = r.conversation_id AND c.user_id = $3
		  AND r.status = 'running'
		RETURNING r.status, r.finished_at`, runID, conversationID, userID, status).
		Scan(&run.Status, &run.FinishedAt)
	if err == nil {
		return run, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return chat.Run{}, fmt.Errorf("finish run: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT r.status, r.finished_at
		FROM buddy.runs r
		JOIN buddy.conversations c ON c.id = r.conversation_id
		WHERE r.id = $1::uuid AND r.conversation_id = $2 AND c.user_id = $3`,
		runID, conversationID, userID).Scan(&run.Status, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return chat.Run{}, fmt.Errorf("run %s: %w", runID, chat.ErrNotFound)
	}
	if err != nil {
		return chat.Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}
