package httpapi

import (
	"context"
	"github.com/nxvrlxv/ai-buddy/backend/internal/absences"
	"net/http"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

type UserStorage interface {
	GetUserByLogin(ctx context.Context, login string) (*auth.User, error)
	GetUserByID(ctx context.Context, userID int64) (*auth.User, error)
}

// TokenManager выпускает и проверяет JWT (реализация — auth.Manager).
type TokenManager interface {
	Issue(user *auth.User) (string, error)
	Parse(token string) (*auth.Claims, error)
}

type Server struct {
	Requests *absences.Service
	users    UserStorage
	tokens   TokenManager
	chat     ChatService  // чаты и история (chat.Service)
	agent    http.Handler // SSE-прокси на агента (NewAgentProxy)
	agentKey []byte       // общий секрет с агентом для /internal/agent/*
}

func New(users UserStorage, tokens TokenManager, chat ChatService, agent http.Handler, agentKey string) *Server {
	return &Server{
		users:    users,
		tokens:   tokens,
		chat:     chat,
		agent:    agent,
		agentKey: []byte(agentKey),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))

	// Чаты для фронта: только видимый текст.
	mux.Handle("POST /api/conversations", s.requireAuth(http.HandlerFunc(s.handleCreateConversation)))
	mux.Handle("GET /api/conversations", s.requireAuth(http.HandlerFunc(s.handleListConversations)))
	mux.Handle("GET /api/conversations/{id}/messages", s.requireAuth(http.HandlerFunc(s.handleListMessages)))

	// Стрим ответа агента. Preflight OPTIONS отвечает cors до requireAuth.
	mux.Handle("POST /api/chat", s.requireAuth(s.agent))

	// Ручки агента: нужен и ключ агента, и токен пользователя.
	NewAgentHandlers(s.chat).Register(mux, func(h http.Handler) http.Handler {
		return s.requireAgentKey(s.requireAuth(h))
	})

	if s.Requests != nil {
		s.Requests.RegisterHTTP(mux, s.requireAuth)
	}
	return cors(mux)
}

// cors: токен идёт в заголовке, не в куках, поэтому "*" безопасен.
// Preflight OPTIONS приходит без токена — отвечаем сразу, до requireAuth.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
