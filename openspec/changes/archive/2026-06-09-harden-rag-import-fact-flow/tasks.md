## 1. 题库导入 Contract 和发布事务

- [x] 1.1 梳理并补充 `internal/questionbank` JSON 导入 contract 测试，覆盖版本化导入包、legacy 数组、wrapped items、字段兼容、中文分隔符和坏类型报错。
- [x] 1.2 增加题库导入 contract 文档或测试 fixture，明确 schema version、source ref、review policy、允许字段形态、归一化结果、字段路径错误和原始值摘要。
- [x] 1.3 将 commit 明确为发布事务，复核或补齐 matched/imported/skipped/embedding synced/embedding failed/failure reasons 的 summary 测试。
- [x] 1.4 复核 review/commit 门禁测试，确认解析、暂存、人工 review、Agent review、重复/脏题阻止、embedding/reindex retry 边界不被绕过。

## 2. RAG Eval 真实 Query 和 Candidate Pool

- [x] 2.1 调研当前 session/RetrievalTrace 持久化入口，确定真实 query 导出的最小事实源。
- [x] 2.2 为 `cmd/rag-eval` 增加真实 query 导出模式，输出脱敏 JSONL，并覆盖邮箱、手机号、URL、secret 片段清洗测试。
- [x] 2.3 为 `cmd/rag-eval` 增加 candidate pool 构建模式，合并 live/stage/keyword/random-negative 候选并保留来源 rank/score。
- [x] 2.4 增加标注输入指标计算或复用现有指标计算路径，覆盖 recall@k、hit@k、MRR 或 nDCG 的回归测试。

## 3. Runtime Retrieval Decision Policy

- [x] 3.1 新增 nodes 层 Runtime Retrieval Decision Policy，输入回答、历史、CandidatePool、RetrievalTrace、已用题、动态难度、技能覆盖和阈值，输出 strategy/include_context/selected/consumed/reason/degraded reason。
- [x] 3.2 在 `retrieve_rag` 或相关 query 构造中传递已用题排除条件，并在 `pick_next` 前再次过滤已用候选。
- [x] 3.3 接入 `pick_next` 和追问链路，使低信息回答、弱召回、正常深挖、候选为空 fallback 和 end 策略都记录可诊断原因。
- [x] 3.4 增加节点级和 graph loop 回归测试，覆盖低信息+高置信、低信息+弱召回、正常回答+可用召回、动态难度/技能覆盖参与、已用题排除、旧 Session 兼容。

## 4. 上线观测和后续运维边界

- [x] 4.1 为 RAG decision、zero-hit、fallback、embedding failure、schema error 明确 trace/log/eval 输出位置，避免后续 observability 旁路实现。
- [x] 4.2 在设计和代码边界中预留 quota/cost guard、Admin operations 和脱敏数据保留接入点，但不实现前端 UI。

## 5. 文档和验证

- [x] 5.1 更新 `docs/SDD-Backend.md`，记录版本化题库导入 contract、发布事务、RAG eval 真实 query 工具链、Runtime Retrieval Decision Policy 和上线运维边界。
- [x] 5.2 按项目规则新增或更新 `docs/code-changes/MM-DD-*.md`，基于真实 diff 记录运行时行为变化。
- [x] 5.3 运行最小相关测试：`go test ./internal/questionbank ./internal/nodes ./cmd/rag-eval -count=1`。
- [x] 5.4 运行全量验证：`go test ./...` 和必要的 `openspec validate harden-rag-import-fact-flow --strict`。
