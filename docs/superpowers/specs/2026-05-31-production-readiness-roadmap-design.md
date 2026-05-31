# Production Readiness Roadmap Design

## Purpose

Move Interview Agent from a locally demonstrable AI interview system to an engineering-business usable system with clear verification, measurable RAG quality, a complete candidate training loop, and minimum deployability and diagnostics.

This roadmap is intentionally staged. Each stage must produce independently testable software and a clean commit. The stages are sequential because later work depends on earlier gates:

1. Verification must be stable before expanding quality metrics.
2. RAG quality must be measurable before using it as a stronger product loop.
3. The product loop must be stable before deployment and tracing become meaningful.
4. Deployment diagnostics must describe real business paths, not isolated demos.

## Current Baseline

The repository already has:

- Go backend with Gin HTTP API, Graph-driven interview flow, LLM/RAG abstraction, PG/Redis optional runtime dependencies, metrics, backpressure, SSE, and session persistence.
- React/Vite frontend for candidate interview, report, question bank, import review, and admin workspace.
- Offline tools for RAG evaluation, question bank linting, mock evaluation, reindexing, structured demos, smoke, load, and chaos scripts.
- Windows-friendly Makefile targets and README commands using `mingw32-make` and PowerShell.
- Recent commits have consolidated large engineering changes, added RAG hard gates, aligned README commands, and covered frontend navigation state.

The repository still lacks a single production-readiness path. README lists remaining gaps: OTel tracing, Helm chart, preview plan, real-scale RAG evaluation, and long-running load evidence.

## Non-Goals

- Do not introduce real LLM/API-key requirements into default local or CI gates.
- Do not require PG/Redis for default verification.
- Do not implement a full RAGAS or TruLens-style framework before the local golden-query evaluator is stronger.
- Do not build a large Kubernetes platform. Helm, if added, must be minimal and runnable.
- Do not redesign the Graph engine or Session aggregate unless a stage directly requires a small compatible field.
- Do not push commits.

## Stage 1: Production Verification Gate

### Goal

Make the default engineering gate explicit and repeatable. A contributor should be able to run one command before commit and get the same checks CI runs for dependency-free paths.

### Scope

- Add `verify-local` Makefile target.
- Add or align CI workflow for dependency-free checks.
- Document the verification contract in README.
- Ensure the gate uses Windows-compatible commands.

### Verification Contract

Default gate should run:

- `go test ./... -count=1`
- `npm --prefix web run test`
- `npm --prefix web run build`
- `mingw32-make eval-rag`
- `mingw32-make questionbank-lint`
- `mingw32-make eval-mock`
- `git diff --check`

CI can use equivalent shell commands on Linux where appropriate, but repository docs must keep Windows/PowerShell first because the local environment is Windows.

### Acceptance Criteria

- `mingw32-make verify-local` passes locally without PG, Redis, Docker, or API keys.
- CI workflow exists and does not depend on secrets or external services.
- README has one clear "before commit" command.
- No production behavior changes.

## Stage 2: RAG Quality Group Gates

### Goal

Make RAG evaluation actionable. A single global recall score can hide weak skill, scenario, or tag groups. The evaluator must show which groups are weak and support gates that fail with useful diagnostics.

### Scope

- Extend `cmd/rag-eval` summary output with grouped metrics.
- Group by fields already present in golden cases and/or retrieved question metadata: `skill`, `scenario`, and `tag`.
- Add `worst_groups` to the summary and report.
- Add threshold flags only where the seed/golden data supports stable gates.
- Keep default `mingw32-make eval-rag` stable.

### Data Shape

`summary.json` should keep existing top-level metrics and add:

```json
{
  "groups": {
    "skill:redis": {
      "cases": 8,
      "recall_at_5": 0.75,
      "recall_at_10": 0.875,
      "mrr_at_k": 0.91,
      "ndcg_at_k": 0.8
    }
  },
  "worst_groups": [
    {
      "group": "skill:redis",
      "metric": "recall_at_5",
      "value": 0.75,
      "threshold": 0.8
    }
  ]
}
```

Exact names may follow existing Go naming conventions, but the output must be machine-readable and stable.

### Acceptance Criteria

- RAG report identifies worst groups.
- Gate failure points to group names, not only global metrics.
- Existing global RAG gates still pass.
- Tests cover grouped aggregation and gate failure formatting.

## Stage 3: Candidate Training Product Loop

### Goal

Close the business loop from interview to next training action. The product should not stop at a report; it should generate a concrete next practice path using the current report and question bank.

### Scope

- Add a preview-plan or equivalent service path that turns report weaknesses and RAG candidates into a next practice plan.
- Keep the first implementation compatible with current Session and Report fields.
- Make report actions explicit: jump to question bank, start a drill, or preview next question set.
- Add frontend regression coverage for start interview, answer submit, report drill, and question jump paths.

### Product Flow

1. Candidate enters resume and JD.
2. Candidate starts interview.
3. Candidate submits answers.
4. System produces report.
5. Report shows training plan with recommended question IDs and skills.
6. Candidate can jump to question bank or start a drill with prefilled weak-skill training context.

### Acceptance Criteria

- Mock-mode demo can exercise the full candidate path.
- Frontend tests cover the critical state transitions without requiring a browser server.
- Question bank links preserve selected question ID.
- The implementation does not require real LLM calls in tests.

## Stage 4: Deployment And Diagnostics Loop

### Goal

Make a deployed instance diagnosable and minimally deployable. When an interview is slow or degraded, operators should find the request path across HTTP, Graph, LLM, parser, and retriever boundaries.

### Scope

- Add minimal tracing hook, preferably OTel-compatible, while preserving existing trace id behavior.
- Add spans around HTTP request handling, Graph execution, LLM calls, parser calls, and RAG retrieval.
- Keep tracing optional and disabled or no-op by default.
- Add minimal deployment templates or health-check docs for local Docker/Compose and optional Helm.
- Standardize load and chaos report locations and README references.

### Acceptance Criteria

- Trace id remains visible in logs and responses where currently expected.
- Tests cover no-op tracing and at least one span-recording path with a fake recorder/exporter.
- Deployment template or docs include liveness/readiness endpoints.
- Load/chaos report paths are documented and stable.

## Cross-Stage Rules

- Use TDD for behavior changes.
- Keep each stage to a coherent commit boundary.
- Do not mix product, deployment, and evaluator work in the same commit unless a stage explicitly requires it.
- Update README only when commands or user workflow change.
- Generated frontend assets must be committed only when frontend build output changes.
- Every stage must end with `git status --short` clean after commit.

## Final Verification

After all four stages:

- `mingw32-make verify-local`
- `mingw32-make eval-rag`
- `mingw32-make e2e-smoke`
- `npm --prefix web run test`
- `npm --prefix web run build`
- `go test ./... -count=1`

Integration tests with PG/Redis remain optional and environment-gated unless the stage explicitly modifies those paths.

## Risks

- CI may require Linux command variants even though local docs are Windows-first. Mitigation: keep Makefile targets portable where practical and document local vs CI command differences.
- Group-level RAG gates may be noisy if a group has too few cases. Mitigation: require minimum group case count before enforcing thresholds.
- Preview-plan work may tempt schema churn. Mitigation: first derive from existing report and question IDs; add fields only if tests prove current data is insufficient.
- Tracing can create dependency bloat. Mitigation: hide it behind a small internal interface and keep the default no-op path.
