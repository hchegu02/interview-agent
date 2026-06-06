## ADDED Requirements

### Requirement: 长期记忆写入应产生结构化可观测信号

系统 MUST 在长期记忆沉淀过程中记录结构化可观测信号，覆盖成功、跳过、失败和冲突重试结果，同时保持面试完成响应不被长期记忆写入失败阻断。

#### Scenario: 长期记忆写入成功

- **WHEN** 面试完成后长期记忆合并并保存成功
- **THEN** 系统 MUST 记录包含 `user_id`、`session_id` 和结果状态的结构化信号

#### Scenario: 长期记忆写入被跳过

- **WHEN** 面试完成但缺少 `user_id`、Report 或长期记忆 Store
- **THEN** 系统 MUST 记录跳过原因
- **AND** 面试完成响应 MUST 保持不变

#### Scenario: 长期记忆写入失败

- **WHEN** 长期记忆 Store 返回非冲突错误
- **THEN** 系统 MUST 记录错误类别和目标用户
- **AND** 面试完成响应 MUST 保持不变

#### Scenario: 长期记忆 CAS 冲突重试耗尽

- **WHEN** 长期记忆写入因 CAS 冲突重试后仍失败
- **THEN** 系统 MUST 记录冲突次数和最终失败状态
- **AND** 面试完成响应 MUST 保持不变

### Requirement: 长期记忆观测信号不得泄露敏感正文

系统 MUST 限制长期记忆观测信号内容，避免记录完整回答正文、完整报告正文、token、密钥或私有配置。

#### Scenario: 写入失败日志不包含完整画像正文

- **WHEN** 长期记忆写入失败并产生日志或事件
- **THEN** 观测信号 MUST NOT 包含完整回答正文或完整报告正文
- **AND** 观测信号 SHOULD 只包含用户、会话、状态、错误类别和计数字段
