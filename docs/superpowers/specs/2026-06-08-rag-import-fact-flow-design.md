---
comet_change: harden-rag-import-fact-flow
role: technical-design
canonical_spec: openspec
---

# RAG Import Fact Flow Design

## Context

Interview Agent 已经具备题库导入、review、commit、embedding、RAG 检索、query rewrite、HyDE shadow、RAG eval 和 Agent Graph 面试链路。当前短板不是单点能力缺失，而是上线事实流不够硬：导入 contract 没有版本化，commit 的发布事务语义不完整，RAG eval 缺少真实 query 闭环，出题/追问节点还没有统一 Runtime Retrieval Decision Policy。

本设计以业务上线使用为目标，而不是以最小工程量为目标。实现可以分阶段，但目标架构必须支持可运营、可回滚、可诊断、可评测。

## Goals

- 建立版本化题库导入 contract，同时兼容现有 `[]Item` 和 `{items:[...]}` 输入。
- 将 import commit 明确为发布事务，保留 review、质量门禁、embedding/reindex 和 summary 事实。
- 扩展 RAG eval，使真实 session/retrieval trace 可以导出脱敏 query，并生成 candidate pool 供人工标注和策略回归。
- 在 nodes 层引入 Runtime Retrieval Decision Policy，统一处理低信息回答、弱召回、已用题排除、动态难度、技能覆盖和上下文注入策略。
- 为后续 quota/cost guard、Admin operations、observability 和脱敏数据保留预留后端边界。

## Non-Goals

- 不改前端题库工作台，不做 UI 美化。
- 不切换 Qdrant，不替换 BGE-M3/pgvector。
- 不把参考项目 Java/Vue 代码移植到本项目。
- 第一阶段不实现完整额度系统、运营面板或用户管理，只设计后端接入边界。

## Architecture

目标事实流：

```text
source material / JSON
  -> Import Package v1 / legacy JSON adapter
  -> contract validation
  -> staged import items
  -> human + agent review
  -> publish commit transaction
  -> question_bank fact source
  -> embedding sync / reindex retry
  -> runtime retrieval
  -> runtime retrieval decision policy
  -> pick_next / probe
  -> retrieval trace + eval export
```

正式题库仍使用现有 `questionbank.Item` 和 `question_bank` 存储。Import Package v1 是导入边界，不是新的题库实体模型。

## Components

### Import Contract

位置：`internal/questionbank`

新增或固化 contract：

- `schema_version`
- `source_ref`
- `items`
- `validation_report`
- `review_policy`
- 字段路径级错误
- 原始值摘要或原始 item 快照

兼容输入：

- `[]Item`
- `{ "items": [...] }`
- 未来版本化包 `{ "schema_version": "...", "items": [...] }`

兼容字段：

- `difficulty`: number 或数字字符串
- `rubric`: object、string array、string
- `tags`、`expected_points`、`role_tags`、`follow_up_hints`: string array 或分隔字符串

坏类型必须报错，错误包含字段名；能包含字段路径和原始值摘要更好。不能把坏字段静默吞成空值。

### Publish Commit Transaction

位置：`internal/questionbank`

commit 不是简单 upsert，而是发布事务：

- 输入：ready import job 和 staged import items
- 门禁：valid、human accepted、agent review 非阻塞、重复检查、内容质量检查
- 写入：正式 `question_bank`
- 派生：embedding sync
- 输出：commit summary

summary 至少区分：

- matched
- imported
- skipped
- embedding_synced
- embedding_failed
- failure_reasons

embedding 失败不应回滚正式题库事实；应写状态和错误，后续通过 reindex/retry 修复派生索引。

### RAG Eval Real Query Pipeline

位置：`cmd/rag-eval`

新增文件型工具链：

```text
export-queries
  -> build-candidate-pool
  -> human label relevance
  -> calculate metrics
```

导出数据用 JSONL。每条 query 至少包含：

- query_id
- query_text
- role/skill/category scope
- phase
- source reference
- optional retrieval trace reference

导出前必须脱敏：

- email
- phone
- URL
- token/api key/password/secret 片段

candidate pool 合并：

- live retriever topK
- RetrievalTrace stage candidates：vector/text/rule/fusion/rerank
- keyword candidates
- deterministic random negatives

同一 question id 去重，但保留所有 source、rank、score。

### Runtime Retrieval Decision Policy

位置：`internal/nodes`

新增策略组件，不放进 `internal/retriever`。Retriever 继续负责召回排序；面试节点负责业务策略。

输入：

- 当前用户回答
- 当前题和历史 rounds/followups
- `Session.CandidatePool`
- `Session.RetrievalTrace`
- 已用主问题和追问题 ID
- WorkingMemory：skill coverage、dynamic difficulty、degraded reasons
- 阈值：high confidence、min context score、context limit

输出：

- `strategy`: `deepen | remedy | switch_topic | fallback | end`
- `include_context`
- `selected_candidate_ids`
- `consumed_candidate_ids`
- `reason`
- `degraded_reason`

规则第一版使用确定性判断：

- 低信息短语：不会、不知道、不清楚、没做过、不了解等
- 极短回答且无技术信号
- 连续低信息回答
- 最高召回分数低于最低上下文阈值
- 候选池排除已用题后为空

策略语义：

- 低信息 + 高置信：`remedy`，允许少量上下文，做低难度补救追问。
- 低信息 + 弱召回：`switch_topic` 或 `fallback`，不强塞上下文。
- 正常回答 + 可用召回：`deepen`，结合动态难度和覆盖度深挖。
- 已用题全部排除后为空：`fallback` 或 `end`，记录 degraded reason。

### Used Question Exclusion

已用题排除必须双层执行：

- 检索前：`retrieve_rag` 尽量把已用 question ids 放入 retriever query/filter。
- 选择前：`pick_next` / policy 再过滤 `CandidatePool`。

这样即使旧 Session、fallback pool 或 retriever 不支持 exclude，也不会重复出题。

## Data Flow

```text
Import:
raw JSON -> parse contract -> normalize Item -> stage import item
  -> review decisions -> commit transaction -> question_bank -> embedding status

Runtime:
session state -> retrieve_rag -> CandidatePool + RetrievalTrace
  -> policy(answer, pool, trace, memory, used ids)
  -> pick_next/probe strategy
  -> session rounds + diagnostics

Eval:
session/trace source -> sanitized query JSONL
  -> candidate pool JSONL
  -> labeled JSONL
  -> metrics report
```

## Error Handling

- Contract parse errors return field-aware errors and fail the import job.
- Unsupported flexible field types must not become nil silently.
- Commit skips unsafe items instead of failing the whole job when individual staged items fail gates.
- Embedding failure records status and error; reindex handles retry.
- RAG export with no valid query returns empty dataset or clear diagnostic error.
- Policy decision with missing score treats retrieval as weak and records degraded reason.

## Compatibility

- Existing legacy JSON imports remain supported.
- Existing Session JSON must load without new policy fields.
- New runtime diagnostics use `omitempty` or internal trace-only structures.
- No front-end contract changes are required in the first implementation stage.

## Testing Strategy

- `internal/questionbank`: contract success cases, bad type errors, versioned package metadata, commit summary, embedding failure status.
- `cmd/rag-eval`: sanitizer tests, export JSONL tests, candidate pool merge/dedupe/source evidence tests, metrics tests.
- `internal/nodes`: policy unit tests for low-info/high-confidence, low-info/weak, normal/deepen, used-question exclusion, empty pool fallback, old Session compatibility.
- Graph loop tests: ensure `pick_next` and `probe_*` still suspend/resume correctly after policy integration.
- Verification: `go test ./internal/questionbank ./internal/nodes ./cmd/rag-eval -count=1`, then `go test ./...`, then `openspec validate harden-rag-import-fact-flow --strict`.

## Rollout

1. Land import contract and publish transaction tests first.
2. Land RAG eval file pipeline second; it does not affect live runtime.
3. Land policy component behind conservative defaults.
4. Wire policy into `pick_next` and probe path.
5. Update SDD and code-change docs.

Rollback should be possible by disabling policy use and keeping existing candidate selection. Import contract changes are stricter by design; rollback would mean allowing unsafe input again and should require explicit decision.

## Risks

- Stricter contract may reject loose JSON that previously half-worked. This is acceptable if errors are field-aware.
- Real query export can leak sensitive data if sanitizer misses patterns. Default exports must be local artifacts and tests must cover common secrets.
- Policy thresholds may be wrong for some roles. Trace reason and eval metrics are the control loop.
- Commit summary may expose complexity. Keep it structured, not narrative.
