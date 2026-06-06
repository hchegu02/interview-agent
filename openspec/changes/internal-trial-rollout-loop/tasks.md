## 1. OpenSpec And Design

- [x] 1.1 Create proposal, design, delta spec, and task checklist for internal trial rollout.
- [x] 1.2 Run OpenSpec validation for the change and repair artifact issues.

## 2. Rollout Documents

- [x] 2.1 Update the internal trial launch checklist as the single entry point.
- [x] 2.2 Add technical trial Runbook with startup, validation, diagnosis, and rollback steps.
- [x] 2.3 Add business trial Runbook with fixed user flow and feedback collection steps.
- [x] 2.4 Add issue record template with required reproduction and diagnostic fields.
- [x] 2.5 Add product feedback scoring template.
- [x] 2.6 Add Go/No-Go standard for continuing, pausing, rolling back, or expanding trial scope.

## 3. Verification

- [x] 3.1 Verify documentation links and tracked paths.
- [x] 3.2 Run minimal config or docs-related tests if touched.
- [x] 3.3 Run `openspec validate internal-trial-rollout-loop --strict`.

Note: implementation changes are Markdown-only rollout documents. No Go, frontend, config, or script files were modified, so no Go/frontend test was required for 3.2.

## 4. Closeout

- [ ] 4.1 Update code-change documentation if implementation changes code.
- [ ] 4.2 Commit rollout documents and OpenSpec artifacts.
