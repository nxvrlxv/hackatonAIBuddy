package httpapi

// Внутренние ручки для Python-агента: история чатов.
// Все маршруты под /internal/agent, оборачиваются в wrap (requireAgentKey + requireAuth),
// пользователь берётся только из claims токена. JSON везде в snake_case.
//
// Объекты:
//   Conversation: {"id":1,"title":"","last_sequence":3,"created_at":"RFC3339","updated_at":"RFC3339"}
//   Message:      {"id":10,"conversation_id":1,"run_id":"uuid","sequence_number":3,"event_key":"user",
//                  "role":"user|assistant|tool|system","payload":<любой JSON>,"created_at":"RFC3339"}
//   Run:          {"run_id":"uuid","conversation_id":1,"status":"running|completed|failed|interrupted",
//                  "finished_at":"RFC3339"|null}
//   Ошибка:       {"error":"..."}
//
// POST /internal/agent/conversations
//   body (необязательно): {"title":"..."}            → 201 Conversation
// GET  /internal/agent/conversations                  → 200 {"conversations":[Conversation]}
// GET  /internal/agent/conversations/{id}/messages?limit=50&before=<sequence_number>
//   последние limit (1..200, по умолчанию 50) сообщений с sequence_number < before,
//   по возрастанию sequence_number, полный payload  → 200 {"messages":[Message]}
// POST /internal/agent/conversations/{id}/turns
//   body: {"client_message_id":"...","payload":{...}}
//   → 201 {"run_id":"uuid","user_message":Message,"history":[Message],
//          "summary":{"text":"","version":0,"through_sequence":0}}
//   history — последние сообщения ДО user_message (по возрастанию), user_message в неё не входит.
// POST /internal/agent/conversations/{id}/runs/{run_id}/messages
//   body: {"event_key":"...","role":"assistant","payload":{...}}
//   → 201 Message (новая запись) | 200 Message (повтор с тем же event_key, даже после finish)
// POST /internal/agent/conversations/{id}/runs/{run_id}/finish
//   body: {"status":"completed|failed|interrupted"}
//   → 200 Run (если ход уже завершён — 200 с текущим статусом, ничего не меняется)
//
// Коды: 400 — невалидный запрос, 401 — нет claims, 404 — нет чата/хода или он чужой,
// 409 — "duplicate message" | "run in progress" | "run not active", 500 — внутренняя ошибка.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
)

const (
	agentSmallBodyLimit   = 4 << 10 // 4 КБ: title, status
	agentMessageBodyLimit = 1 << 20 // 1 МБ: сообщения с payload
)

// ChatService — то, что ручкам агента нужно от chat.Service.
type ChatService interface {
	CreateConversation(ctx context.Context, userID int64, title string) (chat.Conversation, error)
	ListConversations(ctx context.Context, userID int64) ([]chat.Conversation, error)
	ListMessages(ctx context.Context, in chat.ListMessagesInput) ([]chat.Message, error)
	StartTurn(ctx context.Context, in chat.StartTurnInput) (chat.Turn, error)
	AppendMessage(ctx context.Context, in chat.AppendInput) (chat.Message, bool, error)
	FinishRun(ctx context.Context, userID, conversationID int64, runID, status string) (chat.Run, error)
}

type AgentHandlers struct {
	chat ChatService
}

func NewAgentHandlers(c ChatService) *AgentHandlers {
	return &AgentHandlers{chat: c}
}

// Register вешает ручки на mux; wrap применяется к каждой (ключ агента + токен).
func (h *AgentHandlers) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	if wrap == nil {
		wrap = func(next http.Handler) http.Handler { return next }
	}
	const p = "/internal/agent/conversations"
	mux.Handle("POST "+p, wrap(http.HandlerFunc(h.createConversation)))
	mux.Handle("GET "+p, wrap(http.HandlerFunc(h.listConversations)))
	mux.Handle("GET "+p+"/{id}/messages", wrap(http.HandlerFunc(h.listMessages)))
	mux.Handle("POST "+p+"/{id}/turns", wrap(http.HandlerFunc(h.startTurn)))
	mux.Handle("POST "+p+"/{id}/runs/{run_id}/messages", wrap(http.HandlerFunc(h.appendMessage)))
	mux.Handle("POST "+p+"/{id}/runs/{run_id}/finish", wrap(http.HandlerFunc(h.finishRun)))
}

// --- DTO ---

type agentConversation struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	LastSequence int64     `json:"last_sequence"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type agentMessage struct {
	ID             int64           `json:"id"`
	ConversationID int64           `json:"conversation_id"`
	RunID          string          `json:"run_id"`
	SequenceNumber int64           `json:"sequence_number"`
	EventKey       string          `json:"event_key"`
	Role           string          `json:"role"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
}

type agentSummary struct {
	Text            string `json:"text"`
	Version         int    `json:"version"`
	ThroughSequence int64  `json:"through_sequence"`
}

type agentTurnResponse struct {
	RunID       string         `json:"run_id"`
	UserMessage agentMessage   `json:"user_message"`
	History     []agentMessage `json:"history"`
	Summary     agentSummary   `json:"summary"`
}

type agentRun struct {
	RunID          string     `json:"run_id"`
	ConversationID int64      `json:"conversation_id"`
	Status         string     `json:"status"`
	FinishedAt     *time.Time `json:"finished_at"`
}

type createConversationRequest struct {
	Title string `json:"title"`
}

type startTurnRequest struct {
	ClientMessageID string          `json:"client_message_id"`
	Payload         json.RawMessage `json:"payload"`
}

type appendMessageRequest struct {
	EventKey string          `json:"event_key"`
	Role     string          `json:"role"`
	Payload  json.RawMessage `json:"payload"`
}

type finishRunRequest struct {
	Status string `json:"status"`
}

func toAgentConversation(c chat.Conversation) agentConversation {
	return agentConversation{ID: c.ID, Title: c.Title, LastSequence: c.LastSequence, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func toAgentMessage(m chat.Message) agentMessage {
	return agentMessage{
		ID: m.ID, ConversationID: m.ConversationID, RunID: m.RunID, SequenceNumber: m.SequenceNumber,
		EventKey: m.EventKey, Role: m.Role, Payload: m.Payload, CreatedAt: m.CreatedAt,
	}
}

func toAgentMessages(ms []chat.Message) []agentMessage {
	out := make([]agentMessage, 0, len(ms))
	for _, m := range ms {
		out = append(out, toAgentMessage(m))
	}
	return out
}

// --- ручки ---

func (h *AgentHandlers) createConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	var req createConversationRequest
	if !agentDecode(w, r, agentSmallBodyLimit, true, &req) {
		return
	}
	c, err := h.chat.CreateConversation(r.Context(), userID, req.Title)
	if err != nil {
		agentServiceError(w, "create conversation", userID, err)
		return
	}
	agentWriteJSON(w, http.StatusCreated, toAgentConversation(c))
}

func (h *AgentHandlers) listConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	list, err := h.chat.ListConversations(r.Context(), userID)
	if err != nil {
		agentServiceError(w, "list conversations", userID, err)
		return
	}
	out := make([]agentConversation, 0, len(list))
	for _, c := range list {
		out = append(out, toAgentConversation(c))
	}
	agentWriteJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (h *AgentHandlers) listMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	convID, ok := agentConversationID(w, r)
	if !ok {
		return
	}
	in := chat.ListMessagesInput{UserID: userID, ConversationID: convID}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			agentWriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		in.Limit = n
	}
	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			agentWriteError(w, http.StatusBadRequest, "invalid before")
			return
		}
		in.Before = n
	}
	msgs, err := h.chat.ListMessages(r.Context(), in)
	if err != nil {
		agentServiceError(w, "list messages", userID, err)
		return
	}
	agentWriteJSON(w, http.StatusOK, map[string]any{"messages": toAgentMessages(msgs)})
}

func (h *AgentHandlers) startTurn(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	convID, ok := agentConversationID(w, r)
	if !ok {
		return
	}
	var req startTurnRequest
	if !agentDecode(w, r, agentMessageBodyLimit, false, &req) {
		return
	}
	turn, err := h.chat.StartTurn(r.Context(), chat.StartTurnInput{
		UserID: userID, ConversationID: convID, ClientMessageID: req.ClientMessageID, Payload: req.Payload,
	})
	if err != nil {
		agentServiceError(w, "start turn", userID, err)
		return
	}
	agentWriteJSON(w, http.StatusCreated, agentTurnResponse{
		RunID:       turn.RunID,
		UserMessage: toAgentMessage(turn.UserMessage),
		History:     toAgentMessages(turn.History),
		Summary: agentSummary{
			Text: turn.Summary.Text, Version: turn.Summary.Version, ThroughSequence: turn.Summary.ThroughSequence,
		},
	})
}

func (h *AgentHandlers) appendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	convID, ok := agentConversationID(w, r)
	if !ok {
		return
	}
	var req appendMessageRequest
	if !agentDecode(w, r, agentMessageBodyLimit, false, &req) {
		return
	}
	msg, created, err := h.chat.AppendMessage(r.Context(), chat.AppendInput{
		UserID: userID, ConversationID: convID, RunID: r.PathValue("run_id"),
		EventKey: req.EventKey, Role: req.Role, Payload: req.Payload,
	})
	if err != nil {
		agentServiceError(w, "append message", userID, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	agentWriteJSON(w, status, toAgentMessage(msg))
}

func (h *AgentHandlers) finishRun(w http.ResponseWriter, r *http.Request) {
	userID, ok := agentUserID(w, r)
	if !ok {
		return
	}
	convID, ok := agentConversationID(w, r)
	if !ok {
		return
	}
	var req finishRunRequest
	if !agentDecode(w, r, agentSmallBodyLimit, false, &req) {
		return
	}
	run, err := h.chat.FinishRun(r.Context(), userID, convID, r.PathValue("run_id"), req.Status)
	if err != nil {
		agentServiceError(w, "finish run", userID, err)
		return
	}
	agentWriteJSON(w, http.StatusOK, agentRun{
		RunID: run.ID, ConversationID: run.ConversationID, Status: run.Status, FinishedAt: run.FinishedAt,
	})
}

// --- помощники (свои, чтобы не зависеть от respond.go) ---

func agentUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		// Сюда можно попасть, только если маршрут забыли обернуть в requireAuth.
		agentWriteError(w, http.StatusUnauthorized, "unauthenticated")
		return 0, false
	}
	return claims.UserID, true
}

func agentConversationID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		agentWriteError(w, http.StatusBadRequest, "invalid conversation id")
		return 0, false
	}
	return id, true
}

// agentDecode читает JSON-тело с лимитом. allowEmpty — пустое тело допустимо.
func agentDecode(w http.ResponseWriter, r *http.Request, limit int64, allowEmpty bool, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	err := json.NewDecoder(r.Body).Decode(dst)
	if err == nil || (allowEmpty && errors.Is(err, io.EOF)) {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		agentWriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return false
	}
	agentWriteError(w, http.StatusBadRequest, "invalid json")
	return false
}

// agentServiceError переводит ошибку сервиса в HTTP. Логируем только операцию,
// пользователя и ошибку — без payload и токенов.
func agentServiceError(w http.ResponseWriter, op string, userID int64, err error) {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		agentWriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, chat.ErrDuplicateMessage):
		agentWriteError(w, http.StatusConflict, "duplicate message")
	case errors.Is(err, chat.ErrRunInProgress):
		agentWriteError(w, http.StatusConflict, "run in progress")
	case errors.Is(err, chat.ErrRunNotActive):
		agentWriteError(w, http.StatusConflict, "run not active")
	case errors.Is(err, chat.ErrInvalidInput):
		msg := strings.TrimPrefix(err.Error(), chat.ErrInvalidInput.Error()+": ")
		agentWriteError(w, http.StatusBadRequest, msg)
	default:
		log.Printf("agent %s (user %d): %v", op, userID, err)
		agentWriteError(w, http.StatusInternalServerError, "internal error")
	}
}

func agentWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func agentWriteError(w http.ResponseWriter, status int, msg string) {
	agentWriteJSON(w, status, map[string]string{"error": msg})
}
