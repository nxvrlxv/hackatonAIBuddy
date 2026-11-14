package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
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

func TestTeamMembersTool(t *testing.T) {
	ctx := context.Background()
	sasha := &auth.User{ID: 3, TeamID: 2, Role: "intern"}

	t.Run("ok uses team and user from token", func(t *testing.T) {
		repo := &fakeTeam{members: []buddy.TeamMember{
			{ID: 7, FullName: "Игорь Лебедев", Role: "lead"},
			{ID: 1, FullName: "Анна Смирнова", Role: "mentor", IsMyMentor: true},
		}}
		_, out, err := teamMembers(repo)(ctx, requestFor(t, sasha), TeamMembersInput{})
		if err != nil || len(out.Members) != 2 || !out.Members[1].IsMyMentor || out.Members[0].Role != "lead" {
			t.Fatalf("%+v %v", out, err)
		}
		if repo.teamID != 2 || repo.userID != 3 {
			t.Fatalf("must use claims: team %d user %d", repo.teamID, repo.userID)
		}
	})
	t.Run("empty team is []", func(t *testing.T) {
		_, out, err := teamMembers(&fakeTeam{})(ctx, requestFor(t, sasha), TeamMembersInput{})
		if err != nil || out.Members == nil || len(out.Members) != 0 {
			t.Fatalf("%#v %v", out, err)
		}
	})
	t.Run("db error hidden", func(t *testing.T) {
		_, _, err := teamMembers(&fakeTeam{err: errors.New("db down")})(ctx, requestFor(t, sasha), TeamMembersInput{})
		if err == nil || strings.Contains(err.Error(), "db down") {
			t.Fatal(err)
		}
	})
	t.Run("no claims", func(t *testing.T) {
		_, _, err := teamMembers(&fakeTeam{})(ctx, &mcp.CallToolRequest{}, TeamMembersInput{})
		if !errors.Is(err, mcpserver.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})
	t.Run("register", func(t *testing.T) {
		RegisterTeamMembers(mcp.NewServer(&mcp.Implementation{Name: "t", Version: "v0"}, nil), &fakeTeam{})
	})
}
