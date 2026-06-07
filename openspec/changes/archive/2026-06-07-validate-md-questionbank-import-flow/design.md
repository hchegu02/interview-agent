# Design

## Approach

Use the live backend import path rather than a synthetic unit test:

1. Ensure local PostgreSQL is reachable and has required migrations.
2. Ensure local BGE-M3 OpenAI-compatible embedding endpoint is reachable.
3. Start the Go server with `config/config.yaml`.
4. Upload the Markdown file as `source_type=document`.
5. Poll the import job until it reaches `ready` or `failed`.
6. Review generated items with `accept_complete_valid`.
7. Commit the import.
8. Verify `question_bank` rows and `embedding_status=embedded`.
9. Run retrieval evidence with question-bank query and RAG eval where applicable.

## Expected Behavior

Document-generated valid items initially require human review. After `accept_complete_valid`, complete valid items should become publishable and commit should import them into `question_bank`. With `embedding.mode=real` and local BGE-M3 configured, committed items should receive embeddings during commit.

## Failure Handling

- If PostgreSQL or BGE-M3 is unavailable, record the environment blocker and stop before changing code.
- If import fails because of parser / LLM / review / commit logic, capture the error and fix the backend defect in this change.
- If generated items are low quality but the pipeline works, do not change code; record quality risks for the next quality-gate change.

## Data Safety

This change writes only to the local configured PostgreSQL database. It does not delete existing rows. It may upsert formal question-bank rows if generated question IDs collide.
