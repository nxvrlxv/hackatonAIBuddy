package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

// UserGetter — всё, что нужно whoami от репозитория (реализует buddy.Repository).
type UserGetter interface {
	GetUserByID(ctx context.Context, userID int64) (*auth.User, error)
}

// WhoAmIInput пустой: пользователь берётся из Bearer-токена, а не из аргументов.
type WhoAmIInput struct{}

type WhoAmIOutput struct {
	ID       int64   `json:"id"`
	FullName string  `json:"full_name"`
	Role     string  `json:"role"`
	TeamID   int64   `json:"team_id"`
	Team     string  `json:"team"`             // "Команда Авито Путешествий", а не 3
	Mentor   *string `json:"mentor,omitempty"` // у стажёра — имя наставника
}

// whoami возвращает хендлер инструмента; вынесен отдельно, чтобы тестировать без MCP-сервера.
func whoami(users UserGetter) mcp.ToolHandlerFor[WhoAmIInput, WhoAmIOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, _ WhoAmIInput) (*mcp.CallToolResult, WhoAmIOutput, error) {
		claims, err := mcpserver.ClaimsFromRequest(req)
		if err != nil {
			return nil, WhoAmIOutput{}, fmt.Errorf("get claims: %w", err)
		}
		user, err := users.GetUserByID(ctx, claims.UserID)
		if err != nil {
			return nil, WhoAmIOutput{}, fmt.Errorf("get user: %w", err)
		}

		return nil, WhoAmIOutput{
			ID:       user.ID,
			FullName: user.FullName,
			Role:     user.Role,
			TeamID:   user.TeamID,
			Team:     user.Team,
			Mentor:   user.Mentor,
		}, nil
	}
}

func RegisterWhoAmI(server *mcp.Server, users UserGetter) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "whoami",
		Description: "Возвращает профиль текущего пользователя: id, полное имя, роль, команду (id и название) и наставника, если он есть.",
	}, whoami(users))
}
