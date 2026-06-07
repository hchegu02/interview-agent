# Change: Dedupe Question Generation Review Gate

## Problem

真实业务演练后，本地 `question_bank` 中出现多条 `docq-*` 生成题重复内容，例如多条题干都是“Go 服务如何设计超时、重试和熔断，避免级联故障？”。现有生成质量门禁能拦截同一批 LLM 返回中的重复题，但没有把已存在正式题库、已暂存导入项、以及同一生成 job 的 staged 结果作为全局去重边界。

这会带来三个现实问题：

- 内部试用时题库列表出现大量重复题，业务上不可用。
- RAG 检索结果被重复题污染，影响题目多样性和面试体验。
- Agent review 虽然已有 `auto_approved`、`needs_human_review`、`rejected` 状态，但重复题的阻断原因和提交结果不够可诊断。

## Goals

- 在生成题进入暂存审核流程前，阻止与正式题库或同一暂存 job 中已有题目重复的候选题。
- 在 commit 前再次保护，避免重复题绕过生成阶段进入正式 `question_bank`。
- 对被拦截的重复题保留可解释原因，便于内部团队判断是模型生成问题、来源材料问题还是审核策略问题。
- 保持现有人工审核流程：`needs_human_review` 不经人工接受不得提交，`rejected` 不得提交。

## Non-Goals

- 不做数据库 schema 变更。
- 不改前端页面和交互。
- 不引入新的向量相似度去重服务。
- 不改变现有题库 API 响应结构的必填字段；如需新增诊断字段，必须 `omitempty`。
- 不让 skill/MCP 直接写正式题库。

## Scope

涉及后端模块：

- `internal/questionbank/generation_*`
- `internal/questionbank/imports_*`
- `internal/questionbank/store.go` / `pg_store.go` 现有读写能力
- 相关测试和运行文档

## Success Criteria

- 生成阶段能拦截与正式题库既有题干重复的候选题。
- staged import 内重复题不会同时进入可提交状态。
- commit 阶段不会把重复题写入正式题库。
- 重复题拦截原因能在生成 job 或 import item 上看到。
- `go test ./internal/questionbank -count=1` 通过。
