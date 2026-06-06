## ADDED Requirements

### Requirement: 内部试用身份来源必须区别于开发 fallback

系统 MUST 为内部团队试用提供明确身份来源边界，使试用环境可以从可信内部来源解析当前用户，同时保留开发模式 fallback 但不得把 fallback 描述为生产鉴权。

#### Scenario: 内部试用请求使用可信身份来源

- **WHEN** 服务运行在内部试用配置下
- **AND** 请求包含由可信内部网关或 allowlist 配置提供的当前用户标识
- **THEN** owner resolver MUST 使用该可信身份作为当前用户
- **AND** 业务 handler MUST 继续只依赖统一 resolver 和 authorizer

#### Scenario: 开发 fallback 不作为内部试用默认身份

- **WHEN** 服务运行在内部试用配置下
- **AND** 请求只提供开发模式 `X-User-ID` 或 `owner_user_id`
- **THEN** 系统 MUST NOT 静默把该 fallback 当作生产级身份来源
- **AND** 文档或配置 MUST 明确该路径仅用于本地开发或显式允许的试用调试

#### Scenario: 内部试用身份缺失

- **WHEN** 内部试用请求访问受保护用户资源但无法解析当前用户
- **THEN** 授权边界 MUST 拒绝访问
- **AND** 响应 MUST 使用稳定结构化错误格式
