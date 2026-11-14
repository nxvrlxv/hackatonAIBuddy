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
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/mcpserver"
	"github.com/nxvrlxv/ai-buddy/backend/internal/onboarding"
	"github.com/nxvrlxv/ai-buddy/backend/internal/rag"

	"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
		return fmt.Errorf("load conf error: %w", err)
	}

	jwtManager, err := auth.NewManager(cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("create jwt manager: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create pool error: %w", err)
	}
	defer pool.Close()

	repo, err := buddy.NewRepository(pool, ctx)
	if err != nil {
		return fmt.Errorf("create repository error: %w", err)
	}
	client, err := rag.NewEmbedClient(cfg)
	if err != nil {
		return fmt.Errorf("error creating EmbedClient: %w", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "avito-buddy", Version: "v0.1.0"}, nil)
	svc := rag.NewService(client, repo, 5, 0.6)
	RegisterServerTools(server)
	RegisterSearchDocs(server, svc)
	RegisterWhoAmI(server, repo)
	onboarding.RegisterTools(server, onboarding.NewService(onboarding.NewPostgres(pool)))

	return mcpserver.Run(ctx, mcpserver.Options{
		Addr:            ":" + cfg.Port,
		MCPServer:       server,
		Middleware:      mcpauth.RequireBearerToken(mcpserver.Verifier(jwtManager), nil),
		ShutdownTimeout: 10 * time.Second,
	})
}
