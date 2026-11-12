package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"

	//"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run отделена от main, чтобы defer-ы срабатывали до выхода:
// log.Fatal в main вызывает os.Exit и defer-ы пропускает.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// TODO 2. Загрузить конфиг; при ошибке вернуть её.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load conf error: %w", err)
	}

	// TODO 3. Пул через postgres; defer pool.Close().
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create pool error: %w", err)
	}
	defer pool.Close()

	// TODO 4. Репозиторий buddy (сегодня может быть не нужен tool-у, но пригодится завтра).
	// repo, err := buddy.NewRepository(pool, ctx)
	// if err != nil {
	// 	return fmt.Errorf("create repository error: %w", err)
	// }

	server := mcp.NewServer(&mcp.Implementation{Name: "avito-buddy", Version: "v0.1.0"}, nil)
	RegisterServerTools(server)

	// TODO 6. Зарегистрировать tool ping (или whoami): mcp.AddTool(server, &mcp.Tool{...}, handler).

	return mcpserver.Run(ctx, mcpserver.Options{
		Addr:            ":" + cfg.Port,
		MCPServer:       server,
		Middleware:      nil, // JWT завтра
		ShutdownTimeout: 10 * time.Second,
	})
}
