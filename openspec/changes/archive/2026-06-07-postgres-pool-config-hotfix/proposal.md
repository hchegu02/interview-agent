# Postgres Pool Config Hotfix

## Problem

`internal/config.Config.Postgres` exposes pool controls (`max_conns`, `min_conns`, `max_conn_lifetime`, `health_check_period`) and defaults them, but `cmd/server/deps.go` created the runtime pool with `pgxpool.New(ctx, cfg.PostgresDSN)`.

That call parses only the DSN and leaves pgxpool using its defaults for pool sizing and health checks. Under pressure this is dangerous because operators may believe DB connection limits are enforced when they are not.

## Root Cause

The server dependency wiring never translated `cfg.Postgres` into `pgxpool.Config`.

## Goal

Ensure service startup applies the configured Postgres pool settings before creating the runtime `*pgxpool.Pool`, without changing HTTP APIs, database schema, or question-bank query behavior.

## Non-Goals

- Do not change question-bank OFFSET pagination.
- Do not rewrite question-bank search SQL.
- Do not split `sessions.state_json`.
- Do not add pgvector EXPLAIN diagnostics.
- Do not change events retention or partitioning.
