## 1. OpenSpec and Design

- [x] 1.1 Confirm proposal, high-level design, delta specs and task scope for internal trial readiness.
- [x] 1.2 Produce the Superpowers technical design doc through Comet design handoff.

## 2. Identity Boundary

- [x] 2.1 Add or document internal-trial identity source configuration while preserving dev fallback behavior.
- [x] 2.2 Cover missing identity, mismatched owner and dev fallback behavior with tests or fixtures.

## 3. Real Tool Trial Wiring

- [x] 3.1 Add explicit internal-trial wiring for real read-only GitHub tool client without changing default mock behavior.
- [x] 3.2 Ensure missing real tool configuration produces stable diagnostic state and trace.
- [x] 3.3 Update `project_polish` behavior/spec tests so real, mock and failed tool paths are distinguishable.

## 4. Memory Observability Trial Gate

- [x] 4.1 Ensure internal smoke or verification fixture observes long-term memory success, skipped, failed and conflict-exhausted states.
- [x] 4.2 Confirm memory observation payloads do not include full answers, reports, tokens or private config.

## 5. Internal Trial Smoke and Quality Gates

- [x] 5.1 Add a repeatable internal trial smoke command or script covering interview completion, report, memory observation, Agent project polish and tool trace.
- [x] 5.2 Update `cmd/agent-verify` fixtures or related tests if the smoke needs new stable inputs.
- [x] 5.3 Run Go, frontend, agent-verify, smoke and OpenSpec validation gates.

## 6. Documentation

- [x] 6.1 Update backend/frontend SDD with internal trial boundaries and non-production limits.
- [x] 6.2 Add code-change documentation after implementation based on the real diff.
- [ ] 6.3 Archive the OpenSpec change after verification passes.
