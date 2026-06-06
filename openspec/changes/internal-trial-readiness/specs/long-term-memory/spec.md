## ADDED Requirements

### Requirement: 内部试用 smoke 应覆盖长期记忆观测结果

系统 MUST 在内部团队试用门禁中验证长期记忆写入观测信号，确保完整面试链路中的画像沉淀状态可诊断。

#### Scenario: 内部 smoke 观察长期记忆写入成功

- **WHEN** 内部 smoke 完成带 `user_id` 和 Report 的面试
- **THEN** 验证结果 MUST 能确认长期记忆写入成功观测
- **AND** 观测 MUST 包含稳定状态、目标用户和 session 标识

#### Scenario: 内部 smoke 观察长期记忆跳过或失败

- **WHEN** 内部 smoke 或 fixture 模拟缺少必要输入、Store 不可用、非冲突错误或 CAS 冲突耗尽
- **THEN** 验证结果 MUST 能区分 skipped、failed 和 conflict-exhausted 类状态
- **AND** 面试 completed 响应 MUST 保持不被长期记忆失败阻断

#### Scenario: 内部试用观测不泄露敏感正文

- **WHEN** 内部 smoke 收集长期记忆观测
- **THEN** 观测 payload MUST NOT 包含完整回答正文、完整报告正文、token、密钥或私有配置
