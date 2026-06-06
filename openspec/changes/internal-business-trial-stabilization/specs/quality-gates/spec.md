## MODIFIED Requirements

### Requirement: 本地质量门禁应包含内部试用 smoke

系统 MUST 提供可重复执行的内部试用 smoke 验证，用于判断当前构建是否达到内部团队真实试用门槛。Smoke SHOULD include a business-trial evidence check before maintainers mark a version ready for broader internal business use.

#### Scenario: 内部试用 smoke 覆盖完整业务链路

- **WHEN** 开发者运行内部试用 smoke 命令
- **THEN** smoke MUST 覆盖面试开始、答题推进、报告生成、长期记忆观测、Agent 项目润色和 `tool_trace`
- **AND** 任一关键步骤失败时命令 MUST 返回非 0

#### Scenario: 内部试用 smoke 区分 mock 和真实工具

- **WHEN** smoke 在默认配置下运行
- **THEN** 验证 MUST 能确认工具路径是 deterministic mock 或未启用真实工具
- **AND** smoke MUST NOT 要求默认环境联网

#### Scenario: 内部试用 smoke 校验真实工具配置缺失

- **WHEN** smoke 或 fixture 覆盖真实工具配置缺失路径
- **THEN** 验证 MUST 检查稳定错误类别或 trace 状态
- **AND** 验证 MUST 确认该状态没有被伪装成成功真实调用

#### Scenario: 内部业务试用反馈证据通过 smoke

- **WHEN** 维护者运行默认内部试用 smoke
- **THEN** smoke MUST 校验业务试用反馈 fixture 的关键字段和扩大结论
- **AND** smoke MUST 输出稳定业务试用 marker
- **AND** fixture 缺失、评分越界、脚本未完成却标记可扩大、或阻断项与扩大结论冲突时 smoke MUST 返回非 0

### Requirement: 内部试用前必须运行既有验证门禁

系统 MUST 在内部试用说明中列出并维护当前必跑验证命令。

#### Scenario: 内部试用发布检查

- **WHEN** 维护者准备标记某版本可供内部试用
- **THEN** 检查清单 MUST 包含 Go 测试、前端测试、前端构建、agent-verify tool/memory fixtures、内部试用 smoke、业务反馈证据校验和 OpenSpec strict validation
