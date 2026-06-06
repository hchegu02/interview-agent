## 1. Domain Model

- [x] 1.1 Add `Difficulty` constants and `DifficultyState` to runtime domain model.
- [x] 1.2 Add `DifficultyState` field to `WorkingMemory` with migration-safe defaults.

## 2. Difficulty Node

- [x] 2.1 Implement `NewUpdateDifficultyNode` and options in `internal/nodes`.
- [x] 2.2 Apply high-score, low-score, mid-score and degraded-score rules.
- [x] 2.3 Add unit tests for difficulty initialization, escalation, downgrade, reset and degraded skip.
- [x] 2.4 Add idempotency tests for completed round consumption and replay.

## 3. Graph and Documentation

- [x] 3.1 Wire `update_difficulty` between `update_memory` and `reflection_check`.
- [x] 3.2 Add graph-level test or update existing agent loop test to verify node execution.
- [x] 3.3 Update `docs/SDD-Backend.md` and code-change document.
- [x] 3.4 Run targeted tests, `go test ./... -count=1`, and `openspec validate add-dynamic-difficulty-foundation --strict`.
