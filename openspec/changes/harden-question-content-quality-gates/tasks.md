# Tasks

- [x] Add shared deterministic question content quality classifier.
- [x] Extend generation quality gates to flag dirty question text.
- [x] Add import commit re-check so accepted dirty items do not enter formal question bank.
- [x] Add `pick_next` runtime guard to prefer clean candidates and degrade only when all candidates are dirty.
- [x] Extend `cmd/questionbank-lint` to report dirty content issues.
- [x] Add focused tests for classifier, generation, commit, pick_next, and lint.
- [x] Update backend SDD and code-change documentation.
- [x] Run targeted Go tests and OpenSpec validation.
