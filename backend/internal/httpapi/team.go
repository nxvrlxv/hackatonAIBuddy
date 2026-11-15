package httpapi

import (
	"context"
	"log"
	"net/http"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"
)

// TeamLister — состав команды (реализует buddy.Repository).
type TeamLister interface {
	ListTeamMembers(ctx context.Context, teamID, userID int64) ([]buddy.TeamMember, error)
}

// TeamRoutes — GET /api/team: коллеги пользователя для главной страницы.
// Команда и пользователь берутся из токена, чужую команду так не посмотреть.
type TeamRoutes struct{ repo TeamLister }

func NewTeamRoutes(repo TeamLister) TeamRoutes { return TeamRoutes{repo: repo} }

type teamMemberResponse struct {
	ID         int64  `json:"id"`
	FullName   string `json:"full_name"`
	Role       string `json:"role"`
	IsMyMentor bool   `json:"is_my_mentor"`
}

func (t TeamRoutes) Register(mux *http.ServeMux, wrap func(http.Handler) http.Handler) {
	mux.Handle("GET /api/team", wrap(http.HandlerFunc(t.list)))
}

func (t TeamRoutes) list(w http.ResponseWriter, r *http.Request) {
	claims, err := auth.ClaimsFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	list, err := t.repo.ListTeamMembers(r.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		log.Printf("list team %d for user %d: %v", claims.TeamID, claims.UserID, err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]teamMemberResponse, 0, len(list))
	for _, m := range list {
		out = append(out, teamMemberResponse{ID: m.ID, FullName: m.FullName, Role: m.Role, IsMyMentor: m.IsMyMentor})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}
