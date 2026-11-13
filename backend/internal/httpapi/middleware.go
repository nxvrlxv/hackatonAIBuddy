package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

// requireAuth проверяет Bearer-токен и кладёт claims в контекст запроса.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := strings.Fields(r.Header.Get("Authorization"))
		if len(tokenStr) != 2 || !strings.EqualFold(tokenStr[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.tokens.Parse(tokenStr[1])
		if err != nil {
			// причину не раскрываем клиенту
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		ctx := auth.WithClaims(r.Context(), claims)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAgentKey пускает только агента: X-Agent-Key должен совпасть с AGENT_KEY.
// Сравнение за постоянное время, чтобы ключ нельзя было подобрать по таймингу.
// Пустой ключ на сервере не пускает никого.
func (s *Server) requireAgentKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("X-Agent-Key"))
		if len(s.agentKey) == 0 || subtle.ConstantTimeCompare(got, s.agentKey) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid agent key")
			return
		}
		next.ServeHTTP(w, r)
	})
}
