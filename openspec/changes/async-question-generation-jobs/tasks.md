# Tasks

- [x] Add generation job store abstraction and PG-backed persistence.
- [x] Add async generation service path and background worker scheduling.
- [x] Add HTTP `async=true` behavior returning 202.
- [x] Add migrations and tests for async create/get/stage behavior.
- [x] Update SDD and code-change documentation.
- [x] Run targeted Go tests.
- [x] Fix post-verification import commit gate for generated items that still need human review.
- [x] Add pick_next retrieval-rank guard so same-skill low-rank LLM picks do not override RAG final rank1.
