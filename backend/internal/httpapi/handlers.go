package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
)

type loginRequest struct {
	Login string `json:"login"`
}

type userResponse struct {
	ID       int64   `json:"id"`
	Login    string  `json:"login"`
	FullName string  `json:"full_name"`
	Role     string  `json:"role"`
	TeamID   int64   `json:"team_id"`
	Team     string  `json:"team"`
	Mentor   *string `json:"mentor,omitempty"`
}

type loginResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

func toUserResponse(u *auth.User) userResponse {
	return userResponse{
		ID: u.ID, Login: u.Login, FullName: u.FullName, Role: u.Role,
		TeamID: u.TeamID, Team: u.Team, Mentor: u.Mentor,
	}
}

// handleLogin имитирует корпоративный SSO: вход по логину тестовой учётки, без пароля.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10) // логину хватит 1 КБ
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	login := strings.TrimSpace(req.Login)
	if login == "" {
		writeError(w, http.StatusBadRequest, "login is required")
		return
	}

	user, err := s.users.GetUserByLogin(r.Context(), login)
	if errors.Is(err, auth.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid login")
		return
	}
	if err != nil {
		log.Printf("login %q: %v", login, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := s.tokens.Issue(user)
	if err != nil {
		log.Printf("issue token for user %d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: toUserResponse(user)})
}

// handleMe отдаёт свежие данные пользователя из базы (в токене только id).
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		// Сюда можно попасть, только если маршрут забыли обернуть в requireAuth.
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	user, err := s.users.GetUserByID(r.Context(), claims.UserID)
	if errors.Is(err, auth.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if err != nil {
		log.Printf("get user %d: %v", claims.UserID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}
