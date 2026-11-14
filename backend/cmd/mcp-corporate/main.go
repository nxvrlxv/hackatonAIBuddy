// mcp-corporate — MCP-сервер имитаций корпоративных систем (сейчас: заявки на отгул).
// Отделён от mcp-buddy, чтобы имитацию можно было заменить настоящим API без изменения агента.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nxvrlxv/ai-buddy/backend/internal/absences"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	jwtManager, err := auth.NewManager(cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("create jwt manager: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "avito-corporate", Version: "v0.1.0"}, nil)
	absences.Register(server, absences.New(pool))

	return mcpserver.Run(ctx, mcpserver.Options{
		Addr:            ":" + cfg.Port,
		MCPServer:       server,
		Middleware:      mcpauth.RequireBearerToken(mcpserver.Verifier(jwtManager), nil),
		ShutdownTimeout: 10 * time.Second,
	})
}
