# Fix Document Import Approval Verify

## Scope

Hotfix for `fix-document-import-approval`.

Changed backend behavior:

- Human accept advances generated document import items from `needs_human_review` / `rejected` to `auto_approved`.
- Human reject advances generated document import items to `rejected`.
- Structured imports with empty agent review status remain compatible.

No frontend, HTTP API, database schema, or embedding configuration changes.

## Verification

| Check | Result | Evidence |
|---|---|---|
| Tasks completed | PASS | `openspec/changes/fix-document-import-approval/tasks.md` all checked |
| Changed files match tasks | PASS | Changes are limited to `internal/questionbank`, OpenSpec hotfix artifacts, and code-change docs |
| Related tests | PASS | `go test ./internal/questionbank -count=1` |
| Full Go tests | PASS | `go test ./... -count=1` |
| OpenSpec change validation | PASS | `openspec validate fix-document-import-approval --strict` |
| Whitespace check | PASS | `git diff --check` reported only CRLF warnings |
| Security review | PASS | No new secrets, credentials, external calls, or schema changes |

## Notes

Attempted PG integration command:

```powershell
$env:INTEGRATION="1"; $env:INTERVIEW_POSTGRES_DSN="postgres://interview:interview@localhost:5432/interview?sslmode=disable"; go test ./internal/questionbank -run Integration -count=1
```

Result: package reported `[no tests to run]`; there is no matching integration test name in `internal/questionbank`. PG update path is covered by implementation review and shared store behavior tests; a future hardening step can add a real PG metadata round-trip test.

## Verdict

PASS.
