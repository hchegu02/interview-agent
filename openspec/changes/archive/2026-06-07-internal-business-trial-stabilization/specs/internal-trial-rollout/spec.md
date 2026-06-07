## MODIFIED Requirements

### Requirement: 内部试用必须收集轻量产品反馈

业务试用流程 MUST 收集轻量、可比较的产品价值反馈，而不是只记录自由文本。系统 SHOULD provide a minimal machine-checkable feedback fixture so maintainers can verify that business-trial evidence is present before expanding the trial.

#### Scenario: 完成业务试用反馈

- **WHEN** 业务试用者完成一次完整面试和一次项目润色请求
- **THEN** 反馈模板 MUST 记录题目质量、追问质量、报告可信度、项目润色质量和整体可用性评分
- **AND** 反馈模板 MUST 支持补充简短文字说明

#### Scenario: 业务试用稳定版反馈证据可校验

- **WHEN** 维护者准备把内部试用扩大给更多业务试用者
- **THEN** 系统 MUST 提供本地可复跑的业务反馈证据校验
- **AND** 校验 MUST 覆盖固定脚本完成状态、关键评分、扩大结论和阻断项标记
- **AND** 业务反馈证据 MUST NOT 包含 token、私有仓库、完整简历、完整回答或完整报告

### Requirement: 内部试用必须定义 Go/No-Go 标准

内部试用流程 MUST 定义继续、暂停、回滚和禁止扩大范围的判定标准。业务试用稳定版 MUST be treated as controlled internal expansion, not production launch approval.

#### Scenario: 允许继续业务试用

- **WHEN** 核心验证门禁通过、默认配置不访问公网 GitHub、失败 trace 可诊断且长期记忆失败不阻断面试完成
- **AND** 业务试用反馈证据校验通过
- **THEN** 维护者 MAY 标记当前版本可进入受控业务试用
- **AND** 试用说明 MUST 保持内部试用和非生产边界

#### Scenario: 暂停或回滚试用

- **WHEN** 出现不可复现的关键失败、真实工具失败被伪装成成功、身份边界混乱、验证门禁失败或业务反馈存在阻断项但仍被标记可扩大
- **THEN** 维护者 MUST 暂停扩大试用范围
- **AND** 工具相关问题 MUST 优先回滚到 mock 模式
