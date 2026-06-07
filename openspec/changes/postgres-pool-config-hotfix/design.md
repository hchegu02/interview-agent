# Design

## Fix

Create a small helper in `cmd/server/deps.go`:

```go
func postgresPoolConfig(cfg *config.Config) (*pgxpool.Config, error)
```

The helper:

1. Calls `pgxpool.ParseConfig(cfg.PostgresDSN)`.
2. Copies `cfg.Postgres.MaxConns`.
3. Copies `cfg.Postgres.MinConns`.
4. Copies `cfg.Postgres.MaxConnLifetime`.
5. Copies `cfg.Postgres.HealthCheckPeriod`.

`buildAppDeps` then calls `pgxpool.NewWithConfig(ctx, poolCfg)` instead of `pgxpool.New(ctx, cfg.PostgresDSN)`.

## Verification

Add a unit test that does not connect to Postgres. It constructs a config with explicit pool settings, calls `postgresPoolConfig`, and asserts that the pgxpool config contains the expected values.

Run:

```powershell
go test ./cmd/server -count=1
```

## Risk

If an environment sets invalid zero or negative pool values, those values now reach pgxpool instead of being ignored. Defaults are positive. Follow-up hardening can add validation in `internal/config.validate`.
