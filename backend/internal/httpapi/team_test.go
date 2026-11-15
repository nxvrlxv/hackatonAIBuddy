package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"
)

type fakeTeam struct {
	members        []buddy.TeamMember
	err            error
	teamID, userID int64
}

func (f *fakeTeam) ListTeamMembers(_ context.Context, teamID, userID int64) ([]buddy.TeamMember, error) {
	f.teamID, f.userID = teamID, userID
	return f.members, f.err
}

func serveTeam(t *testing.T, repo TeamLister, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewTeamRoutes(repo).Register(mux, func(h http.Handler) http.Handler { return h })
	req := httptest.NewRequest(http.MethodGet, "/api/team", nil)
	if claims != nil {
		req = req.WithContext(auth.WithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestTeamList(t *testing.T) {
	repo := &fakeTeam{members: []buddy.TeamMember{
		{ID: 1, FullName: "Игорь Лидов", Role: "lead"},
		{ID: 2, FullName: "Анна Смирнова", Role: "mentor", IsMyMentor: true},
	}}
	rec := serveTeam(t, repo, &auth.Claims{UserID: 3, TeamID: 2})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if repo.teamID != 2 || repo.userID != 3 {
		t.Fatalf("ids from token: team %d user %d", repo.teamID, repo.userID)
	}
	var body struct {
		Members []teamMemberResponse `json:"members"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Members) != 2 || body.Members[1].FullName != "Анна Смирнова" || !body.Members[1].IsMyMentor {
		t.Fatalf("members: %+v", body.Members)
	}
}

func TestTeamEmptyIsArray(t *testing.T) {
	rec := serveTeam(t, &fakeTeam{}, &auth.Claims{UserID: 3, TeamID: 2})
	if got := rec.Body.String(); got != "{\"members\":[]}\n" {
		t.Fatalf("body %q", got)
	}
}

func TestTeamErrors(t *testing.T) {
	if rec := serveTeam(t, &fakeTeam{}, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no claims: %d", rec.Code)
	}
	if rec := serveTeam(t, &fakeTeam{err: errors.New("db down")}, &auth.Claims{UserID: 3, TeamID: 2}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("db error: %d", rec.Code)
	}
}
