## MODIFIED Requirements

### Requirement: Postgres Pool Configuration

When a Postgres DSN is configured, backend entry points that load application `Config` SHALL apply the configured Postgres pool controls to the runtime pgxpool before opening the pool.

#### Scenario: Pool config maps from application config

- **GIVEN** `Config.PostgresDSN` is non-empty
- **AND** `Config.Postgres` sets `MaxConns`, `MinConns`, `MaxConnLifetime`, and `HealthCheckPeriod`
- **WHEN** backend code constructs the Postgres pool configuration through the shared config helper
- **THEN** the resulting pgxpool config contains the configured values
- **AND** the runtime pool is opened with that config

#### Scenario: Server startup uses configured pool

- **GIVEN** the HTTP server is started with a non-empty Postgres DSN
- **WHEN** server dependencies are built
- **THEN** the Postgres pool is opened with the shared configured pgxpool config

#### Scenario: Backend diagnostics and eval use configured pool

- **GIVEN** `cmd/rag-eval`, `cmd/demo`, or `cmd/questionbank-explain` loads application config with a non-empty Postgres DSN
- **WHEN** the command opens a Postgres pool
- **THEN** it MUST use the shared configured pgxpool config

#### Scenario: Invalid pool config fails early

- **GIVEN** application config has an invalid Postgres pool setting
- **WHEN** config validation runs
- **THEN** validation MUST fail before a Postgres pool is opened

#### Scenario: No DSN keeps Postgres disabled

- **GIVEN** `Config.PostgresDSN` is empty
- **WHEN** server dependencies are built
- **THEN** no Postgres pool is created
- **AND** in-memory stores remain available for local/mock operation
