// Package postgres owns PostgreSQL connections and schema compatibility.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.Tracer = measurementTracer{}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database initialization failed")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database connection failed")
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Check(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return errors.New("database unavailable")
	}
	return s.checkHistory(ctx)
}
