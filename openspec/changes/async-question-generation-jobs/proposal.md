# Async Question Generation Jobs

## Why

真实业务演练显示，本地 `gpt-5.5` 题库生成一次 LLM 调用可能耗时约 55 秒。当前 `POST /api/question-bank/generation-jobs` 同步执行完整检索、concept 抽取、候选题生成和质量门禁，容易被 LLM timeout 或 HTTP write timeout 切断。把 timeout 拉长只能缓解本地演练，不能作为长期业务路线。

当前系统已经有题库导入异步 job 模式。题库生成应采用同类 Go worker + 可查询 job 状态，而不是引入 RocketMQ/RQ。

## What Changes

- Add asynchronous generation job creation via `POST /api/question-bank/generation-jobs?async=true`.
- Persist generation job state so `GET /api/question-bank/generation-jobs/:id` survives request completion and can be polled.
- Run the existing generation pipeline in a Go background worker.
- Keep `POST /api/question-bank/generation-jobs` synchronous for compatibility.
- Keep stage behavior review-first: only `created` jobs can be staged into import review flow.

## Non-Goals

- No RocketMQ, RQ, or external queue dependency.
- No frontend changes.
- No automatic commit into formal `question_bank`.
- No production API calls in tests.
