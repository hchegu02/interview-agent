## Context

当前项目已经有两类状态：

- `domain.Session`：单次面试的持久化会话，包含题目、回答、评分、报告和流程断点。
- `domain.WorkingMemory`：单次 Session 内的运行时记忆，供 pick_next、reflect、report 等节点使用。

下一阶段需要跨 Session 的用户画像，但如果直接把长期画像字段继续塞进 `Session`，会混淆“本次面试事实”和“跨会话学习结果”。因此第一版只新增 `internal/memory` 基础层，让后续数据库、动态难度和复习计划都依赖一个稳定接口。

## Goals / Non-Goals

**Goals:**

- 新增 `UserMemory`、`Weakness` 等长期记忆领域模型。
- 新增 `Store` 接口和线程安全内存实现。
- 新增从 `domain.Session` / `domain.Report` 构建 `UserMemoryUpdate` 并合并到 `UserMemory` 的规则函数。
- 保持所有逻辑可单元测试，不依赖 LLM、HTTP、数据库或 Graph runner。
- 更新 SDD，明确第一版已完成基础层但尚未自动接入面试流程。

**Non-Goals:**

- 不新增 HTTP API。
- 不修改 Interview Graph 节点流转。
- 不修改 Session JSON 字段和数据库 schema。
- 不实现 PostgreSQL/MySQL/Redis 长期记忆存储。
- 不让长期记忆覆盖当前 Session 内已经发生的事实。

## Decisions

### 1. 独立 `internal/memory`，不放进 `internal/domain`

长期记忆和面试 Session 有输入关系，但不是 Session 聚合的一部分。独立包可以减少领域模型互相污染，也方便后续替换 Store 实现。

备选方案是把 `UserMemory` 放进 `internal/domain/session.go`。这会让 Session 模型继续膨胀，而且容易让 Graph 节点误把长期画像当成本轮事实使用，因此不选。

### 2. 使用 `UserMemoryUpdate` 表达一次报告的增量

从报告沉淀画像时，先把 `Session` 转成增量，再应用到既有 `UserMemory`。这样 `BuildUpdateFromSession` 只负责解析报告，`ApplyUpdate` 只负责合并，测试边界清楚。

备选方案是 `BuildFromSession` 直接返回完整 `UserMemory`。这会让“新建画像”和“合并历史画像”混在一起，不利于处理多次面试的分数和弱点去重。

### 3. 第一版合并规则确定性优先

规则：

- `Report.Highlights` 合并为 strengths，去重并保序。
- `Report.Improvements` 和低分 `SkillBreakdown` 生成 weaknesses。
- `Weaknesses` 按 `Topic + Evidence` 去重，重复弱点保留更高严重度和最新更新时间。
- `Report.SkillBreakdown` 合并到 `SkillScores`，同名 skill 用新旧分数均值保守更新。
- `Report.NextSteps` 和 `DrillPlan` 合并为 advice。
- `UpdatedAt` 只接受非零且不早于当前画像时间的更新，避免回放旧报告导致画像时间倒退。

备选方案是让 LLM 总结长期画像。现阶段不选，因为可验证性差、成本高，而且会把长期记忆基础层和模型调用耦合。

### 4. 内存 Store 只做开发和测试实现

`MemoryStore` 使用 mutex 保护 map，并在读写时复制数据，避免调用方拿到内部引用后破坏状态。

备选方案是直接接数据库。现阶段不选，因为需要 schema、迁移、失败补偿和部署配置，超出 A 阶段范围。

内存 Store 接口保留 `context.Context` 是为了对齐后续数据库 Store；当前内存实现不监听 cancel。

## Risks / Trade-offs

- [Risk] 规则画像不如 LLM 总结自然 → Mitigation：第一版只作为结构化画像基础，后续可在独立 change 中增加 LLM summarizer。
- [Risk] 均值合并会弱化最近一次面试信号 → Mitigation：保留 `UpdatedAt`，后续可改为带时间衰减或置信度权重。
- [Risk] 内存 Store 重启丢失 → Mitigation：明确只用于开发和测试；数据库 Store 后续单独实现。
- [Risk] 不接入 Graph 导致业务链路暂时不自动沉淀 → Mitigation：先保证模型和合并规则可测，下一阶段再把报告节点或服务层接入 Store。
