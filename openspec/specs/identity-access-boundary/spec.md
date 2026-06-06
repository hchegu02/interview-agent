# identity-access-boundary Specification

## Purpose

定义用户资源访问时的当前用户解析、开发模式回退来源和授权边界，确保业务 handler 依赖统一身份结果而不是直接解析认证细节。

## Requirements

### Requirement: 后端应通过可替换身份来源解析当前用户

系统 MUST 提供后端身份解析边界，用于从请求上下文解析当前用户身份，并允许开发模式 owner header 被生产身份来源替换。

#### Scenario: 生产身份来源解析当前用户

- **WHEN** HTTP 请求已经由上游认证层注入当前用户身份
- **THEN** 后端 MUST 通过统一 resolver 读取当前用户标识
- **AND** 业务 handler MUST NOT 直接解析 JWT、cookie 或第三方认证细节

#### Scenario: 开发模式 header 只作为回退来源

- **WHEN** 系统运行在未接入真实认证的开发模式
- **THEN** resolver MAY 从 `X-User-ID` 或 `owner_user_id` 解析用户标识
- **AND** 文档 MUST 明确该 header 不是生产安全边界

### Requirement: 用户资源访问必须经过授权边界

系统 MUST 在读取或写入用户归属资源前检查当前用户是否有权访问目标用户资源。

#### Scenario: 当前用户读取本人画像

- **WHEN** 当前用户标识与目标 `user_id` 一致
- **THEN** 授权边界 MUST 允许读取该用户长期画像

#### Scenario: 当前用户读取他人画像

- **WHEN** 当前用户标识与目标 `user_id` 不一致
- **THEN** 授权边界 MUST 拒绝访问
- **AND** HTTP API MUST 返回稳定未授权或禁止访问响应

#### Scenario: 缺少当前用户身份

- **WHEN** 请求无法解析当前用户身份
- **THEN** 受保护用户资源 API MUST 拒绝访问
- **AND** 响应 MUST 保持结构化错误格式

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
