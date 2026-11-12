// Package chat — правила хранения чатов агента: чаты, ходы (runs) и сообщения.
// SQL живёт в postgres/buddy, HTTP — в httpapi; здесь только типы, ошибки и проверки.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Ошибки сервиса. Хендлеры переводят их в HTTP-коды (404 / 409 / 400).
var (
	// ErrNotFound — чата или хода нет, либо он принадлежит другому пользователю.
	ErrNotFound = errors.New("not found")
	// ErrDuplicateMessage — ход с таким client_message_id в этом чате уже был.
	ErrDuplicateMessage = errors.New("duplicate message")
	// ErrRunInProgress — в чате уже есть ход в статусе running.
	ErrRunInProgress = errors.New("run in progress")
	// ErrRunNotActive — запись в завершённый или протухший ход.
	ErrRunNotActive = errors.New("run not active")
	// ErrInvalidInput — некорректные входные данные; текст после ":" безопасно отдавать клиенту.
	ErrInvalidInput = errors.New("invalid input")
)

// Статусы хода.
const (
	StatusRunning     = "running"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

// Роли сообщений (совпадают с CHECK в buddy.messages).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleSystem    = "system"
)

// UserEventKey — event_key сообщения пользователя, которое вставляется при старте хода.
const UserEventKey = "user"

// StaleAfter — ход без активности дольше этого считается прерванным.
const StaleAfter = 5 * time.Minute

type Conversation struct {
	ID           int64
	Title        string
	LastSequence int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Message struct {
	ID             int64
	ConversationID int64
	RunID          string
	SequenceNumber int64
	EventKey       string
	Role           string
	Payload        json.RawMessage
	CreatedAt      time.Time
}

type Run struct {
	ID             string
	ConversationID int64
	Status         string
	FinishedAt     *time.Time
}

// Summary — резюме чата. Пока SUMMARY_ENABLED=false, всегда пустое.
type Summary struct {
	Text            string
	Version         int
	ThroughSequence int64
}

// Turn — результат старта хода.
type Turn struct {
	RunID       string
	UserMessage Message
	// History — последние сообщения ДО нового user-сообщения, по возрастанию sequence_number.
	History []Message
	Summary Summary
}

type StartTurnInput struct {
	UserID          int64
	ConversationID  int64
	ClientMessageID string
	Payload         json.RawMessage
	// HistoryLimit выставляет сервис, значение от вызывающего игнорируется.
	HistoryLimit int
}

type AppendInput struct {
	UserID         int64
	ConversationID int64
	RunID          string
	EventKey       string
	Role           string
	Payload        json.RawMessage
}

type ListMessagesInput struct {
	UserID         int64
	ConversationID int64
	Limit          int
	// Before — вернуть сообщения с sequence_number < Before; 0 — с конца.
	Before int64
}

// Store — хранилище (реализует buddy.Repository). Все методы ограничены user_id.
type Store interface {
	CreateConversation(ctx context.Context, userID int64, title string) (Conversation, error)
	ListConversations(ctx context.Context, userID int64) ([]Conversation, error)
	ListMessages(ctx context.Context, in ListMessagesInput) ([]Message, error)
	StartTurn(ctx context.Context, in StartTurnInput) (Turn, error)
	// AppendMessage возвращает created=false, если это повтор с тем же event_key.
	AppendMessage(ctx context.Context, in AppendInput) (msg Message, created bool, err error)
	FinishRun(ctx context.Context, userID, conversationID int64, runID, status string) (Run, error)
}
