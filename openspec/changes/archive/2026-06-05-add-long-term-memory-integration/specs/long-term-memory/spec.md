## MODIFIED Requirements

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
