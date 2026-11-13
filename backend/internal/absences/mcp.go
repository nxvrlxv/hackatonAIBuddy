package absences

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

type Empty struct{}
type TypesOutput struct {
	Types []Type `json:"types"`
}
type ListOutput struct {
	Requests []Request `json:"requests"`
}

func Register(server *mcp.Server, s *Service) {
	mcp.AddTool(server, &mcp.Tool{Name: "get_request_types", Description: "Получить доступные типы заявок и требования к полям перед оформлением."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, TypesOutput, error) {
			if _, err := mcpserver.ClaimsFromRequest(r); err != nil {
				return nil, TypesOutput{}, err
			}
			v, e := s.Types(ctx)
			return nil, TypesOutput{v}, safe(e)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "list_my_requests", Description: "Посмотреть последние 100 своих заявок и их реальные статусы."},
		func(ctx context.Context, r *mcp.CallToolRequest, _ Empty) (*mcp.CallToolResult, ListOutput, error) {
			c, e := mcpserver.ClaimsFromRequest(r)
			if e != nil {
				return nil, ListOutput{}, e
			}
			v, e := s.List(ctx, c.UserID)
			return nil, ListOutput{v}, safe(e)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "create_time_off_request", Description: "Создать заявку на отгул на указанные целые дни включительно. Только по явной просьбе оформить; submitted не означает согласование."},
		func(ctx context.Context, r *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Request, error) {
			c, e := mcpserver.ClaimsFromRequest(r)
			if e != nil {
				return nil, Request{}, e
			}
			v, e := s.Create(ctx, c.UserID, in)
			return nil, v, safe(e)
		})
}
func safe(err error) error {
	if err == nil || errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
		return err
	}
	return errors.New("request service unavailable")
}
