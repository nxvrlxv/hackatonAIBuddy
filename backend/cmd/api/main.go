package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nxvrlxv/ai-buddy/backend/internal/absences"
	"github.com/nxvrlxv/ai-buddy/backend/internal/auth"
	"github.com/nxvrlxv/ai-buddy/backend/internal/chat"
	"github.com/nxvrlxv/ai-buddy/backend/internal/config"
	"github.com/nxvrlxv/ai-buddy/backend/internal/httpapi"
	"github.com/nxvrlxv/ai-buddy/backend/internal/onboarding"
	"github.com/nxvrlxv/ai-buddy/backend/internal/postgres/buddy"
)

// historyLimit — сколько последних сообщений агент получает в начале хода.
const historyLimit = 50

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

	tokens, err := auth.NewManager(cfg.JWTSecret, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("create jwt manager: %w", err)
	}

	agent, err := newAgentProxy(cfg.AgentURL, cfg.AgentKey)
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	defer pool.Close()

	repo, err := buddy.NewRepository(pool, ctx)
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}

	chats := chat.NewService(repo, historyLimit)
	checklist := onboarding.NewHandlers(onboarding.NewService(onboarding.NewPostgres(pool)))

	requests := absences.New(pool)
	server := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.New(repo, tokens, chats, agent, cfg.AgentKey).
			WithRoutes(checklist, requests).
			Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout не ставим: /api/chat отдаёт длинный SSE-поток
	}
	return serve(ctx, server, 10*time.Second)
}

// newAgentProxy проверяет AGENT_URL и собирает SSE-прокси на агента.
func newAgentProxy(rawURL, agentKey string) (http.Handler, error) {
	target, err := url.Parse(rawURL)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("AGENT_URL must be a valid HTTP(S) URL")
	}
	return httpapi.NewAgentProxy(target, agentKey), nil
}

// serve запускает сервер и блокируется до отмены ctx, затем ждёт текущие запросы.
// nil — штатная остановка.
func serve(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	errs := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()
	log.Printf("api listening on %s", server.Addr)

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
