package httpapi

// Публичные ручки чатов для фронта. Все за requireAuth, пользователь — из claims токена.
//
// POST /api/conversations   body (необязательно): {"title":"..."} → 201 Conversation
// GET  /api/conversations                                      → 200 {"conversations":[Conversation]}
// GET  /api/conversations/{id}/messages?limit=50&before=<seq>  → 200 {"messages":[VisibleMessage]}
//
// Фронту отдаём только видимый текст: роли user и assistant с непустым content.
// Вызовы инструментов, ответы tool и system-сообщения скрыты.
// limit применяется к сырым сообщениям до фильтрации, поэтому видимых может прийти меньше.

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
)

type visibleMessage struct {
	ID             int64     `json:"id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	SequenceNumber int64     `json:"sequence_number"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := claimsUserID(w, r)
	if !ok {
		return
	}
	var req createConversationRequest
	if !agentDecode(w, r, agentSmallBodyLimit, true, &req) {
		return
	}
	c, err := s.chat.CreateConversation(r.Context(), userID, req.Title)
	if err != nil {
		chatError(w, "create conversation", userID, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAgentConversation(c))
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := claimsUserID(w, r)
	if !ok {
		return
	}
	list, err := s.chat.ListConversations(r.Context(), userID)
	if err != nil {
		chatError(w, "list conversations", userID, err)
		return
	}
	out := make([]agentConversation, 0, len(list))
	for _, c := range list {
		out = append(out, toAgentConversation(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := claimsUserID(w, r)
	if !ok {
		return
	}
	convID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || convID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	in := chat.ListMessagesInput{UserID: userID, ConversationID: convID}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		if in.Limit, err = strconv.Atoi(v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
	}
	if v := q.Get("before"); v != "" {
		if in.Before, err = strconv.ParseInt(v, 10, 64); err != nil {
			writeError(w, http.StatusBadRequest, "invalid before")
			return
		}
	}
	msgs, err := s.chat.ListMessages(r.Context(), in)
	if err != nil {
		chatError(w, "list messages", userID, err)
		return
	}
	out := make([]visibleMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != chat.RoleUser && m.Role != chat.RoleAssistant {
			continue
		}
		text := visibleText(m.Payload)
		if text == "" {
			continue
		}
		out = append(out, visibleMessage{
			ID: m.ID, Role: m.Role, Content: text, SequenceNumber: m.SequenceNumber, CreatedAt: m.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out})
}

// visibleText достаёт текст из payload в формате OpenAI: {"role","content","tool_calls"?}.
// content — строка или массив частей [{"type":"text","text":"..."}]. Иначе — "".
func visibleText(payload json.RawMessage) string {
	var p struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || len(p.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(p.Content, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(p.Content, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func claimsUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		// Сюда можно попасть, только если маршрут забыли обернуть в requireAuth.
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return 0, false
	}
	return claims.UserID, true
}

// chatError переводит ошибку chat в HTTP. В лог — только операция, пользователь и ошибка.
func chatError(w http.ResponseWriter, op string, userID int64, err error) {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, chat.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), chat.ErrInvalidInput.Error()+": "))
	default:
		log.Printf("%s (user %d): %v", op, userID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
