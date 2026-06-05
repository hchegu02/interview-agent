## 1. Service Wiring

- [x] 1.1 Add optional long-term memory Store dependency to `InterviewService`.
- [x] 1.2 Add constructor/setter path for injecting memory Store while preserving existing constructors.
- [x] 1.3 Update `cmd/server` interview service wiring to inject `memory.NewMemoryStore()`.

## 2. Completion Integration

- [x] 2.1 Add service-layer helper that builds a memory update from completed Session reports.
- [x] 2.2 Merge the update with existing user memory, treating not-found as a new profile.
- [x] 2.3 Serialize service-layer get/apply/upsert to avoid in-process lost updates.
- [x] 2.4 Call the helper after completed Session save in `Answer`, without changing completed response behavior.
- [x] 2.5 Keep memory write failures non-blocking for the interview completion path.

## 3. Tests and Documentation

- [x] 3.1 Add tests for completed Answer writing long-term memory.
- [x] 3.2 Add tests for missing report or missing memory Store not writing memory.
- [x] 3.3 Add tests for memory Store failure not breaking completed Answer.
- [x] 3.4 Add test for concurrent memory merges preserving both updates.
- [x] 3.5 Update `docs/SDD-Backend.md` and code-change document.
- [x] 3.6 Run targeted tests, `go test ./... -count=1`, and `openspec validate add-long-term-memory-integration --strict`.
