# Tasks

- [ ] Add generation request/response domain types and validation.
- [ ] Implement source chunk retrieval scoped to a document import job.
- [ ] Add query rewriting / evidence retrieval boundary with mock-safe tests.
- [ ] Implement concept card extraction, backend-generated concept IDs, and concept deduplication.
- [ ] Build evidence packs from concept cards and source chunks.
- [ ] Implement structured QuestionCandidate parser and validation gates.
- [ ] Add duplicate, low-value-question, and source-grounding checks before staging.
- [ ] Add versioned generated-question metadata for concept, generation, question type, answer, and source refs.
- [ ] Stage generated questions through existing import review flow.
- [ ] Add HTTP endpoints for generation job create/get/stage.
- [ ] Add focused tests for quality gates and commit blocking.
- [ ] Update backend SDD / code-change docs for the new generation workflow.
