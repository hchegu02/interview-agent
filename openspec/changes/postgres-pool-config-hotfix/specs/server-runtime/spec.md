## ADDED Requirements

### Requirement: Postgres Pool Configuration

When a Postgres DSN is configured, service startup SHALL apply the configured Postgres pool controls to the runtime pgxpool before opening the pool.

#### Scenario: Pool config maps from application config

- **GIVEN** `Config.PostgresDSN` is non-empty
- **AND** `Config.Postgres` sets `MaxConns`, `MinConns`, `MaxConnLifetime`, and `HealthCheckPeriod`
- **WHEN** server dependencies construct the Postgres pool configuration
- **THEN** the resulting pgxpool config contains the configured values
- **AND** the runtime pool is opened with that config

#### Scenario: No DSN keeps Postgres disabled

- **GIVEN** `Config.PostgresDSN` is empty
- **WHEN** server dependencies are built
- **THEN** no Postgres pool is created
- **AND** in-memory stores remain available for local/mock operation
