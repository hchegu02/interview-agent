# RAG Pipeline Rerank Upgrade Design

## Goal

把当前面试系统的 RAG 从“单路题库召回”升级为“可解释、可评估、可回归”的多路检索 pipeline。升级后的系统要支持 vector、BM25、规则召回、RRF 融合和可插拔 rerank，并先通过离线评估证明质量，再接入在线面试出题链路。

这次升级服务两个目标：

- 工程目标：让检索链路有清晰边界、稳定降级、可观测指标和本地验证门槛。
- 简历目标：能实事求是写出多路 RAG、rerank、本地模型服务接入、Recall/MRR/nDCG 评估和 group-level gate。

## Non-Goals

本阶段不做以下内容：

- 不引入 Elasticsearch 或外部搜索集群。
- 不在 Go 进程内直接加载 rerank 模型。
- 不实现完整 MCP 工具层。
- 不实现长期用户记忆系统。
- 不新增 WebSocket；当前 SSE 已满足面试事件流。

这些能力可以作为后续主线，但不应混进本次 RAG 升级。

## Current Baseline

当前项目已经具备：

- `internal/retriever`：pgvector retriever 和 fallback retriever。
- `internal/nodes/retrieve_rag`：面试 Graph 中的 RAG 召回节点。
- `cmd/rag-eval`：离线 RAG 评估，输出 Recall@5/10、MRR、nDCG、group metrics 和硬门槛。
- `Makefile verify-local`：本地无外部依赖验证门禁。
- `Session.CandidatePool`：保存本轮面试候选题。

主要缺口：

- 检索目前不是多路召回，精确术语类 query 容易依赖向量召回单点表现。
- eval 只评最终结果，无法解释 vector、BM25、rule、RRF、rerank 每一阶段贡献。
- rerank 没有可插拔接口和本地 HTTP 服务协议。
- 在线面试链路没有保存召回来源、融合分和 rerank 排序变化。

## Architecture

新增 `RetrievalPipeline`，把检索拆成可测试的阶段：

```text
Query
  -> Query Analyzer
  -> Vector Retriever
  -> BM25 Retriever
  -> Rule Retriever
  -> RRF Fusion
  -> Optional Reranker
  -> Final Candidates + RetrievalTrace
```

### Components

#### Vector Retriever

复用现有 pgvector / seed fallback 逻辑。它负责语义召回，不承担关键词精确匹配和融合职责。

#### BM25 Retriever

基于题库文本、标签、技能类目和 expected points 构建本地 BM25 索引。第一版不接外部搜索服务，避免部署复杂度。

BM25 需要单测覆盖：

- tokenizer 对大小写、短 token、技术术语的处理。
- IDF 和 term frequency 计算。
- 同分排序稳定性。
- 空 query 和空 corpus 的行为。

#### Rule Retriever

根据结构化字段召回候选题：

- `QuestionBankFilter.SkillCategories`
- `QuestionBankFilter.Scenarios`
- `DifficultyMin` / `DifficultyMax`
- JD/简历分析得到的关键技能
- 题库 tags

规则召回不是为了单路指标最高，而是为了保证关键技能、难度和业务约束不会被向量召回漏掉。

#### RRF Fusion

使用 Reciprocal Rank Fusion 合并 vector、BM25、rule 结果：

```text
score(doc) = sum(1 / (k + rank_i(doc)))
```

同一题目需要去重，并保留每一路来源证据：

- source name
- original rank
- original score
- fused score
- matched tags / rule reason

#### Reranker

定义 Go 内部接口：

```go
type Reranker interface {
    Rerank(ctx context.Context, req RerankRequest) (RerankResult, error)
}
```

支持三种模式：

- `disabled`：直接返回 RRF 排序，默认本地和 CI 使用。
- `mock`：确定性规则打分，用于测试 rerank 流程。
- `local_http`：调用本地 rerank 服务，例如 `http://127.0.0.1:9001/rerank`。

本地 HTTP 请求：

```json
{
  "query": "Go GMP 调度模型为什么快？",
  "top_k": 10,
  "candidates": [
    {
      "id": "go-gmp-001",
      "text": "讲一下 Go GMP 调度模型...",
      "metadata": {
        "skill": "go",
        "difficulty": "4"
      },
      "features": {
        "rrf_score": 0.72,
        "vector_score": 0.81,
        "bm25_score": 4.2
      }
    }
  ]
}
```

本地 HTTP 响应：

```json
{
  "model": "bge-reranker-local",
  "items": [
    { "id": "go-gmp-001", "score": 0.93 }
  ]
}
```

Rerank 调用必须有 timeout。默认失败策略：

- 在线面试：rerank 失败降级到 RRF，并记录降级原因。
- 离线 eval：默认降级；可通过 flag 切成失败模式。
- CI：使用 `disabled` 或 `mock`，不依赖本地模型服务。

## Data Model

新增 `RetrievalTrace`，保存一次检索的审计信息。

建议挂在 `domain.Session` 上，而不是塞进 `WorkingMemory`：

```go
type RetrievalTrace struct {
    Query string
    Stages []RetrievalStageTrace
    Final []RetrievalCandidateTrace
    Rerank *RerankTrace
    FallbackReasons []string
}
```

理由：检索证据是会话事实，服务于报告、前端解释和排障；`WorkingMemory` 应继续表达 Agent 策略状态，不要混入审计数据。

每个候选题至少记录：

- question id
- final rank
- final score
- source stages
- vector score
- BM25 score
- rule reason
- RRF score
- rerank score

## Offline Evaluation

`cmd/rag-eval` 从“评最终结果”升级为“评每个阶段贡献”。

新增输出：

```json
{
  "stages": {
    "vector": { "recall_at_5": 0.72, "mrr_at_k": 0.88 },
    "bm25": { "recall_at_5": 0.61, "mrr_at_k": 0.70 },
    "rule": { "recall_at_5": 0.48, "mrr_at_k": 0.55 },
    "rrf": { "recall_at_5": 0.80, "mrr_at_k": 0.91 },
    "rerank": { "recall_at_5": 0.82, "mrr_at_k": 0.94 }
  },
  "stage_deltas": {
    "rrf_vs_vector_recall_at_5": 0.08,
    "rerank_vs_rrf_mrr_at_k": 0.03
  }
}
```

新增 flags：

- `-stage vector|bm25|rule|rrf|rerank|all`
- `-min-stage-recall-at-5 rrf=0.75,rerank=0.78`
- `-min-stage-mrr-at-k rerank=0.90`
- `-min-delta rrf_vs_vector_recall_at_5=0.03`
- `-emit-failures 10`
- `-explain-case <case_id>`

门槛策略：

- 不要求 BM25 或 rule 单路指标高。
- 默认门槛放在 RRF 和 rerank 的最终效果上。
- group-level gate 继续保留，防止总体指标掩盖局部退化。
- stage delta 用于证明多路融合相比 vector 至少不退化，必要时要求最小提升。

## Online Integration

`retrieve_rag` 节点改为调用 `RetrievalPipeline.Search(ctx, query, filter)`。

输出：

- `Session.CandidatePool` 保存最终候选题。
- `Session.RetrievalTrace` 保存检索证据。
- 降级原因写入 trace，并在必要时同步到 `WorkingMemory.DegradedReasons`。

前端和报告可以展示：

- 推荐题来自哪些召回通道。
- RRF 融合分。
- rerank 前后排序变化。
- 哪些 tags / skill / difficulty 触发了规则召回。
- rerank 失败时是否降级到 RRF。

## Observability

新增或补齐指标：

- `interview_rag_stage_retrieve_total`
- `interview_rag_stage_retrieve_duration_seconds_bucket`
- `interview_rag_stage_candidates_total`
- `interview_rag_rerank_total`
- `interview_rag_rerank_errors_total`
- `interview_rag_rerank_fallback_total`
- `interview_rag_rerank_duration_seconds_bucket`

指标 label 需要低基数：

- `stage`
- `status`
- `mode`

不要把 query、question id、错误文本直接作为 label。

## Error Handling

在线链路的失败策略：

- vector 失败：继续 BM25/rule；如果全部失败，返回 fallback 题库或明确错误。
- BM25 失败：记录 stage error，继续其他召回。
- rule 失败：记录 stage error，继续其他召回。
- RRF 无候选：返回空候选并记录 degraded reason。
- rerank timeout/error：降级到 RRF。

离线 eval 的失败策略：

- 默认允许 rerank 降级，保证本地 CI 稳定。
- 提供严格模式，让本地 rerank 服务异常时直接失败，便于调试真实模型服务。

## Testing

### Unit Tests

- BM25 tokenizer、IDF、排序、空输入。
- Rule retriever 对 skill、scenario、difficulty、tags 的过滤。
- RRF 去重、融合分、同分稳定排序。
- Reranker disabled/mock/local_http timeout/error。
- RetrievalTrace 的来源证据和排序变化。

### Command Tests

- `cmd/rag-eval` stage metrics。
- `stage_deltas` 计算。
- stage-level gate failure。
- `-explain-case` 输出指定 case 的检索证据。

### Integration / Smoke

- `mingw32-make eval-rag`
- `mingw32-make verify-local`
- `mingw32-make e2e-smoke`

默认 CI 不依赖本地 rerank 服务。

## Rollout Plan

### Stage 1: RAG Pipeline Base

- 新增 pipeline 结构。
- 实现 BM25 retriever。
- 实现 rule retriever。
- 实现 RRF fusion。
- 新增 RetrievalTrace。
- 保持现有面试链路默认行为可运行。

### Stage 2: Offline Eval Upgrade

- `cmd/rag-eval` 输出 stage metrics。
- 增加 stage deltas 和 stage gates。
- 增加 `-explain-case`。
- 扩充 golden queries，补技术术语 case。

### Stage 3: Local HTTP Reranker

- 新增 Reranker 接口。
- 实现 disabled/mock/local_http。
- 增加 config、timeout、fallback、metrics。
- 文档化本地 HTTP 协议。

### Stage 4: Online Integration and Explainability

- `retrieve_rag` 接入 RetrievalPipeline。
- `Session` 保存 RetrievalTrace。
- 前端/报告展示召回来源、融合分、rerank 变化。
- README 增加多路 RAG 使用和简历亮点说明。

## Resume Positioning

升级完成后，简历可以写：

> 设计并实现 AI 模拟面试官系统的多路 RAG 检索链路，基于 Go 构建 vector/BM25/rule 召回、RRF 融合和本地 HTTP rerank 服务接入，并通过 Recall@K、MRR、nDCG、stage delta 和 group-level gate 建立离线质量回归体系；将同一检索 pipeline 接入模拟面试 Graph，实现可解释的个性化出题和 rerank 失败自动降级。

