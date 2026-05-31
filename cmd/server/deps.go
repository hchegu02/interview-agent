package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/config"
)

type appDeps struct {
	PGPool *pgxpool.Pool
}

func buildAppDeps(ctx context.Context, cfg *config.Config) (appDeps, func(), error) {
	if cfg.PostgresDSN == "" {
		return appDeps{}, func() {}, nil
	}
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return appDeps{}, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return appDeps{}, nil, fmt.Errorf("ping postgres: %w", err)
	}
	return appDeps{PGPool: pool}, pool.Close, nil
}
