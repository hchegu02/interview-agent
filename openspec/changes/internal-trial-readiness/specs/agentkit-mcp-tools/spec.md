## ADDED Requirements

### Requirement: 内部试用真实工具必须显式启用

系统 MUST 保持默认 deterministic mock 工具行为，并只在内部试用配置显式启用时装配真实只读工具 client。

#### Scenario: 默认装配不访问真实 GitHub

- **WHEN** 服务未配置内部试用真实 GitHub 工具
- **THEN** 默认 Agent 工具注册 MUST 使用 deterministic mock
- **AND** 系统 MUST NOT 默认联网访问 GitHub

#### Scenario: 内部试用显式启用真实 GitHub 工具

- **WHEN** 内部试用环境显式配置真实 GitHub 工具 client 所需参数
- **THEN** `github.project_analyze` MUST 仍通过 `ToolRegistry.Call` 执行
- **AND** 调用 MUST 经过权限、schema、超时和 before/after hook

#### Scenario: 真实 GitHub 工具配置缺失

- **WHEN** 内部试用环境请求真实 GitHub 工具但配置不完整
- **THEN** 工具调用 MUST 返回稳定 `config_missing` 或等价错误类别
- **AND** 系统 MUST NOT 回退为伪装成功的真实调用
