## Why

动态难度状态机已经写入 `WorkingMemory.Difficulty`，但 RAG 检索仍使用静态 `RetrieveRAGOptions.TargetDifficulty` 作为基础目标难度。结果是连续高分/低分只能改变 Session 状态，不能影响下一题候选池，业务效果不闭环。

## What Changes

- `retrieve_rag` 读取 `WorkingMemory.Difficulty.Current` 作为 RAG 基础目标难度。
- 将运行时三档难度映射到题库 1-5 难度：easy -> 2，medium -> 3，hard -> 4。
- 保留现有 `GapStrategy` 微调逻辑。
- 保留 `QuestionBankFilter.DifficultyMin/DifficultyMax` 作为用户显式硬过滤条件。
- 缺少动态难度状态或状态非法时回退到静态 `TargetDifficulty`，兼容旧 Session。

## Capabilities

### Modified Capabilities

- `dynamic-difficulty`: 动态难度不再只停留在工作记忆中，而是会影响下一次 RAG 候选题目标难度。

## Impact

- 影响代码：`internal/nodes/retrieve_rag.go`、`internal/nodes/setup_test.go`。
- 影响文档：更新 `docs/SDD-Backend.md`。
- 不改变 HTTP API、数据库 schema、前端类型或 Session 顶层字段。
- 无新增第三方依赖。
