package buddy

import (
	"context"
	"fmt"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
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
