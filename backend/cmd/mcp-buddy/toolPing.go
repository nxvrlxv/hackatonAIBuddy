package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// пустая структура тк нам пох это просто проверка
type PingInput struct{}

type PingOutput struct {
	Status string `json:"status" jsonschema:"статус ответа сервера"`
	Name   string `json:"name" jsonschema:"имя сервера"`
}

func ping(_ context.Context, _ *mcp.CallToolRequest, _ PingInput) (*mcp.CallToolResult, PingOutput, error) {
	return nil, PingOutput{Status: "ok", Name: "avito-buddy"}, nil
}

func RegisterServerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Description: "Утилита ping, возвращает статус и имя сервера",
		Name:        "ping",
	}, ping)
}
