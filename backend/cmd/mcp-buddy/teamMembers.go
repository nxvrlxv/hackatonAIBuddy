package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
	"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"
)

// TeamLister — то, что инструменту нужно от репозитория (реализует buddy.Repository).
type TeamLister interface {
	ListTeamMembers(ctx context.Context, teamID, userID int64) ([]buddy.TeamMember, error)
}

type TeamMembersInput struct{}

type TeamMemberOut struct {
	FullName   string `json:"full_name"`
	Role       string `json:"role"` // lead | mentor | intern
	IsMyMentor bool   `json:"is_my_mentor"`
}

type TeamMembersOutput struct {
	Members []TeamMemberOut `json:"members"`
}

func teamMembers(repo TeamLister) mcp.ToolHandlerFor[TeamMembersInput, TeamMembersOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, _ TeamMembersInput) (*mcp.CallToolResult, TeamMembersOutput, error) {
		claims, err := mcpserver.ClaimsFromRequest(req)
		if err != nil {
			return nil, TeamMembersOutput{}, fmt.Errorf("get claims: %w", err)
		}
		list, err := repo.ListTeamMembers(ctx, claims.TeamID, claims.UserID)
		if err != nil {
			return nil, TeamMembersOutput{}, errors.New("не удалось получить состав команды")
		}
		out := TeamMembersOutput{Members: make([]TeamMemberOut, 0, len(list))} // не nil, иначе в JSON будет null
		for _, m := range list {
			out.Members = append(out.Members, TeamMemberOut{FullName: m.FullName, Role: m.Role, IsMyMentor: m.IsMyMentor})
		}
		return nil, out, nil
	}
}

func RegisterTeamMembers(server *mcp.Server, repo TeamLister) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "team_members",
		Description: "Состав команды текущего пользователя: коллеги, их роли (lead — руководитель, mentor — наставник, intern — стажёр) " +
			"и пометка is_my_mentor. Используй на вопросы о команде, коллегах, наставнике и к кому обратиться.",
	}, teamMembers(repo))
}
