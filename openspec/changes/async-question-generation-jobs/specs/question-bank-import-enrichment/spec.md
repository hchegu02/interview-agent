# question-bank-import-enrichment Delta

## ADDED Requirements

### Requirement: 定向题库生成支持异步任务

系统 MUST support asynchronous targeted question generation so long-running real LLM calls do not require the HTTP request to remain open until generation completes.

#### Scenario: 异步创建生成任务

- **WHEN** caller posts to `/api/question-bank/generation-jobs?async=true` with a valid generation request
- **THEN** system MUST create a generation job with status `queued`
- **AND** system MUST return HTTP 202 with the queued job
- **AND** system MUST execute generation in a backend worker

#### Scenario: 轮询异步生成任务状态

- **WHEN** caller gets `/api/question-bank/generation-jobs/:id`
- **THEN** system MUST return the latest persisted job status
- **AND** status MUST eventually become `created` or `failed` after worker completion

#### Scenario: 未完成任务不能暂存

- **WHEN** caller posts `/api/question-bank/generation-jobs/:id/stage`
- **AND** the generation job is not `created`
- **THEN** system MUST reject staging
- **AND** system MUST NOT create import review items
