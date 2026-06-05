## 1. Memory Model and Store

- [x] 1.1 Add `internal/memory` package with `UserMemory`, `Weakness`, `UserMemoryUpdate` and errors.
- [x] 1.2 Define `Store` interface with `GetUserMemory` and `UpsertUserMemory`.
- [x] 1.3 Implement thread-safe in-memory Store with defensive copies.

## 2. Report-to-Memory Rules

- [x] 2.1 Implement `BuildUpdateFromSession` to extract strengths, weaknesses, skill scores and advice from `domain.Session`.
- [x] 2.2 Implement `ApplyUpdate` to merge an update into existing `UserMemory`.
- [x] 2.3 Keep merge logic deterministic, deduplicated and independent from LLM/HTTP/Graph.

## 3. Tests and Documentation

- [x] 3.1 Add unit tests for Store save/read/not-found behavior.
- [x] 3.2 Add unit tests for report-to-memory update and merge behavior.
- [x] 3.3 Update `docs/SDD-Backend.md` to mark Long-term Memory基础层为已落地，并明确未接入 Graph/DB/API。
- [x] 3.4 Add code-change document for this code change.
- [x] 3.5 Run targeted memory tests, `go test ./... -count=1` and `openspec validate add-long-term-memory --strict`.
