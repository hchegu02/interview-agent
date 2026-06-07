# Design

## Approach

Use the existing Go backend as the worker host. A generation request can be accepted as queued, persisted in a generation job store, and executed by a bounded background goroutine. This solves HTTP timeout without introducing a separate message broker.

```text
POST /generation-jobs?async=true
  -> validate request
  -> create GenerationJob(status=queued)
  -> persist job
  -> schedule worker
  -> HTTP 202

worker
  -> run existing Generate pipeline
  -> update status/progress in job store
  -> final status created or failed

GET /generation-jobs/:id
  -> read persisted/in-memory job

POST /generation-jobs/:id/stage
  -> require status=created and candidates present
  -> stage into import review flow
```

## Storage

Add a `GenerationJobStore` abstraction. In PG-backed server mode, use a PostgreSQL table storing the whole `GenerationJob` as JSON plus indexed status timestamps. In memory mode, keep the existing map behavior.

The first implementation persists the job JSON as the source of truth instead of splitting concepts/candidates into separate tables. This is acceptable because generation jobs are operational workflow state, while formal questions still go through the existing import review and `question_bank` tables.

## Compatibility

The existing synchronous endpoint remains unchanged unless `async=true` is supplied. Existing clients that expect `201` and a completed job continue to work. Async callers receive `202` and poll `GET`.

## Failure Handling

Worker failures mark the job as `failed` with the error message. Stage rejects jobs without accepted candidates or jobs that are not yet `created`. Shutdown does not cancel already running LLM calls forcibly, but the job state remains inspectable.
