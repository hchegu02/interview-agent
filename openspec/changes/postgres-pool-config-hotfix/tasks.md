## 1. Root Cause

- [x] 1.1 Confirm `Config.Postgres` defines pool settings.
- [x] 1.2 Confirm server startup used `pgxpool.New(ctx, cfg.PostgresDSN)` and ignored those settings.

## 2. Fix

- [x] 2.1 Add helper to parse DSN and copy configured pool settings into `pgxpool.Config`.
- [x] 2.2 Create the runtime pool with `pgxpool.NewWithConfig`.
- [x] 2.3 Preserve no-DSN behavior.

## 3. Verification

- [x] 3.1 Add a unit test for Postgres pool config mapping without connecting to a real database.
- [x] 3.2 Run `go test ./cmd/server -count=1`.
- [x] 3.3 Update `docs/code-changes/MM-DD-*.md`.
