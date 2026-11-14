package onboarding

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

type GetChecklistInput struct{}

type UpdateTaskStatusInput struct {
	TaskID int64  `json:"task_id" jsonschema:"id задачи из get_checklist"`
	Status string `json:"status" jsonschema:"новый статус: todo, in_progress или done"`
}

// RegisterTools добавляет инструменты чек-листа. Пользователь всегда берётся из токена.
func RegisterTools(server *mcp.Server, svc *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_checklist",
		Description: "Чек-лист адаптации текущего стажёра: этапы, задачи с id и статусами, общий прогресс и следующий шаг (next). " +
			"Используй на вопросы «что мне делать дальше», «какой у меня прогресс», «что осталось». " +
			"У наставников и лидов чек-листа нет — список будет пустым.",
	}, getChecklist(svc))

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_task_status",
		Description: "Меняет статус задачи из чек-листа адаптации и возвращает задачу с новым прогрессом. " +
			"task_id бери из get_checklist. Вызывай, когда пользователь сообщает, что выполнил (done) или начал (in_progress) задачу.",
	}, updateTaskStatus(svc))
}

func getChecklist(svc *Service) mcp.ToolHandlerFor[GetChecklistInput, Checklist] {
	return func(ctx context.Context, req *mcp.CallToolRequest, _ GetChecklistInput) (*mcp.CallToolResult, Checklist, error) {
		claims, err := mcpserver.ClaimsFromRequest(req)
		if err != nil {
			return nil, Checklist{}, fmt.Errorf("get claims: %w", err)
		}
		c, err := svc.Checklist(ctx, claims.UserID)
		if err != nil {
			return nil, Checklist{}, errors.New("не удалось получить чек-лист")
		}
		return nil, c, nil
	}
}

func updateTaskStatus(svc *Service) mcp.ToolHandlerFor[UpdateTaskStatusInput, UpdateResult] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in UpdateTaskStatusInput) (*mcp.CallToolResult, UpdateResult, error) {
		claims, err := mcpserver.ClaimsFromRequest(req)
		if err != nil {
			return nil, UpdateResult{}, fmt.Errorf("get claims: %w", err)
		}
		res, err := svc.SetStatus(ctx, claims.UserID, in.TaskID, Status(in.Status))
		switch {
		case err == nil:
			return nil, res, nil
		// Понятные модели сообщения: по ним она может исправить вызов.
		case errors.Is(err, ErrInvalidStatus):
			return nil, UpdateResult{}, errors.New("неверный статус: допустимы todo, in_progress, done")
		case errors.Is(err, ErrNotFound):
			return nil, UpdateResult{}, errors.New("задача не найдена в чек-листе пользователя; возьми task_id из get_checklist")
		default:
			return nil, UpdateResult{}, errors.New("не удалось обновить задачу")
		}
	}
}
