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
	poolCfg, err := postgresPoolConfig(cfg)
	if err != nil {
		return appDeps{}, nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return appDeps{}, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return appDeps{}, nil, fmt.Errorf("ping postgres: %w", err)
	}
	return appDeps{PGPool: pool}, pool.Close, nil
}

func postgresPoolConfig(cfg *config.Config) (*pgxpool.Config, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.Postgres.MaxConns
	poolCfg.MinConns = cfg.Postgres.MinConns
	poolCfg.MaxConnLifetime = cfg.Postgres.MaxConnLifetime
	poolCfg.HealthCheckPeriod = cfg.Postgres.HealthCheckPeriod
	return poolCfg, nil
}
