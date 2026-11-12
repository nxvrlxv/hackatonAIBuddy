package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefaultHistoryLimit = 50
	DefaultMessageLimit = 50
	MaxMessageLimit     = 200
	MaxTitleLength      = 200
	MaxKeyLength        = 200
)

// Service проверяет входные данные и делегирует работу хранилищу.
type Service struct {
	store        Store
	historyLimit int
}

// NewService создаёт сервис. historyLimit <= 0 означает DefaultHistoryLimit.
func NewService(store Store, historyLimit int) *Service {
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	return &Service{store: store, historyLimit: historyLimit}
}

func invalid(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, msg)
}

func (s *Service) CreateConversation(ctx context.Context, userID int64, title string) (Conversation, error) {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > MaxTitleLength {
		return Conversation{}, invalid("title is too long")
	}
	return s.store.CreateConversation(ctx, userID, title)
}

func (s *Service) ListConversations(ctx context.Context, userID int64) ([]Conversation, error) {
	return s.store.ListConversations(ctx, userID)
}

func (s *Service) ListMessages(ctx context.Context, in ListMessagesInput) ([]Message, error) {
	if in.Limit == 0 {
		in.Limit = DefaultMessageLimit
	}
	if in.Limit < 0 || in.Limit > MaxMessageLimit {
		return nil, invalid(fmt.Sprintf("limit must be between 1 and %d", MaxMessageLimit))
	}
	if in.Before < 0 {
		return nil, invalid("before must be non-negative")
	}
	return s.store.ListMessages(ctx, in)
}

func (s *Service) StartTurn(ctx context.Context, in StartTurnInput) (Turn, error) {
	in.ClientMessageID = strings.TrimSpace(in.ClientMessageID)
	if in.ClientMessageID == "" {
		return Turn{}, invalid("client_message_id is required")
	}
	if len(in.ClientMessageID) > MaxKeyLength {
		return Turn{}, invalid("client_message_id is too long")
	}
	if err := validatePayload(in.Payload); err != nil {
		return Turn{}, err
	}
	in.HistoryLimit = s.historyLimit
	turn, err := s.store.StartTurn(ctx, in)
	if err != nil {
		return Turn{}, err
	}
	turn.Summary = Summary{} // SUMMARY_ENABLED=false
	if turn.History == nil {
		turn.History = []Message{}
	}
	return turn, nil
}

func (s *Service) AppendMessage(ctx context.Context, in AppendInput) (Message, bool, error) {
	if !IsUUID(in.RunID) {
		return Message{}, false, invalid("run_id must be a uuid")
	}
	in.EventKey = strings.TrimSpace(in.EventKey)
	if in.EventKey == "" {
		return Message{}, false, invalid("event_key is required")
	}
	if len(in.EventKey) > MaxKeyLength {
		return Message{}, false, invalid("event_key is too long")
	}
	switch in.Role {
	case RoleUser, RoleAssistant, RoleTool, RoleSystem:
	default:
		return Message{}, false, invalid("role must be one of user, assistant, tool, system")
	}
	if err := validatePayload(in.Payload); err != nil {
		return Message{}, false, err
	}
	return s.store.AppendMessage(ctx, in)
}

func (s *Service) FinishRun(ctx context.Context, userID, conversationID int64, runID, status string) (Run, error) {
	if !IsUUID(runID) {
		return Run{}, invalid("run_id must be a uuid")
	}
	switch status {
	case StatusCompleted, StatusFailed, StatusInterrupted:
	default:
		return Run{}, invalid("status must be one of completed, failed, interrupted")
	}
	return s.store.FinishRun(ctx, userID, conversationID, runID, status)
}

func validatePayload(p json.RawMessage) error {
	trimmed := bytes.TrimSpace(p)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return invalid("payload is required")
	}
	if !json.Valid(trimmed) {
		return invalid("payload must be valid json")
	}
	return nil
}

// IsUUID проверяет каноническую запись UUID (8-4-4-4-12, hex), чтобы не слать мусор в ::uuid.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}
