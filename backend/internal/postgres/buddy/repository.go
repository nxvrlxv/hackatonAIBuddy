package buddy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB — то, что репозиторию нужно от пула. Его реализуют *pgxpool.Pool и pgxmock (в тестах).
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ DB = (*pgxpool.Pool)(nil)

type Repository struct {
	pool DB
}

func NewRepository(pool *pgxpool.Pool, ctx context.Context) (*Repository, error) {

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("error, database ureachable: %w", err)
	}

	return &Repository{
		pool: pool,
	}, nil
}

// NewRepositoryFromDB оборачивает произвольный DB без проверки соединения (для тестов).
func NewRepositoryFromDB(db DB) *Repository {
	return &Repository{pool: db}
}
