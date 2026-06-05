# long-term-memory Specification

## Purpose
TBD - created by archiving change add-long-term-memory. Update Purpose after archive.
## Requirements
### Requirement: 长期记忆应表达跨会话用户画像

系统 MUST 提供长期记忆领域模型，用于表达跨 Session 的用户强项、弱点、技能分数、复习建议和更新时间。

#### Scenario: 用户画像包含核心字段

- **WHEN** 系统创建或读取用户长期记忆
- **THEN** 长期记忆 MUST 包含 `user_id`、`strengths`、`weaknesses`、`skill_scores`、`last_advice` 和 `updated_at`

### Requirement: 长期记忆 Store 应支持读写用户画像

系统 MUST 提供长期记忆 Store 接口，并提供可用于开发和测试的内存实现。

#### Scenario: 保存并读取用户画像

- **WHEN** 调用方保存某个用户的长期记忆
- **THEN** 后续按同一 `user_id` 读取 MUST 返回保存后的画像

#### Scenario: 未找到用户画像

- **WHEN** 调用方读取不存在的 `user_id`
- **THEN** Store MUST 返回可通过 `errors.Is` 判断的 not found 错误

### Requirement: 单次面试报告应可沉淀为长期记忆增量

系统 MUST 提供从 `domain.Session` / `domain.Report` 构建长期记忆增量的规则函数，并在面试完成后把已保存且包含 Report 的 Session 沉淀到长期记忆 Store。

#### Scenario: 报告生成画像增量

- **WHEN** Session 包含 `user_id` 和终评报告
- **THEN** 系统 MUST 从报告的 highlights、improvements、skill breakdown、next steps 和 drill plan 中生成长期记忆增量

#### Scenario: 缺少必要输入

- **WHEN** Session 为空、`user_id` 为空或报告缺失
- **THEN** 长期记忆增量规则函数 MUST 返回错误，而不是生成不完整长期记忆

#### Scenario: 面试完成后自动沉淀画像

- **WHEN** 面试 Answer 流程推进到 completed 且 Session 已保存
- **THEN** 系统 MUST 将该 Session 的 Report 合并写入长期记忆 Store

#### Scenario: 面试完成但报告缺失时跳过沉淀

- **WHEN** 面试 Answer 流程推进到 completed 但 Session 没有 Report
- **THEN** 系统 MUST 跳过长期记忆写入
- **AND** 系统 MUST 保持面试完成响应和 Session 保存结果不变

#### Scenario: 长期记忆写入失败不阻断面试完成

- **WHEN** 面试已完成但长期记忆 Store 写入失败
- **THEN** 系统 MUST 保持面试完成响应和 Session 保存结果不变

### Requirement: 长期记忆合并不应覆盖 Session 事实

系统 MUST 只把报告结果合并进 `UserMemory`，不得修改输入 `domain.Session`、`domain.Report` 或 `WorkingMemory`。

#### Scenario: 合并画像保持 Session 不变

- **WHEN** 系统把一次面试报告增量应用到长期记忆
- **THEN** 输入 Session 和 Report MUST 保持不变
- **AND** 只有目标 `UserMemory` 被更新

