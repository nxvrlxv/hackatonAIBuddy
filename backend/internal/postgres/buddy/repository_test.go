package buddy

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Пул подключается лениво, поэтому ошибка видна только на Ping внутри NewRepository.
func TestNewRepositoryUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/db?connect_timeout=1&sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(pool, ctx)
	if err == nil || repo != nil {
		t.Fatalf("expected ping error, got repo=%v err=%v", repo, err)
	}
}

func TestNewRepositoryFromDB(t *testing.T) {
	db, repo := newMockRepo(t)
	if repo.pool != db {
		t.Fatal("repository must use the given DB")
	}
}
