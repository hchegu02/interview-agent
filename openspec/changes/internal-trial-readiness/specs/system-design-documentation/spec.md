## ADDED Requirements

### Requirement: SDD 必须声明内部试用边界和非生产限制

项目 MUST 在系统设计文档中明确内部团队试用能力边界，避免把内部试用、mock 工具、开发 fallback 或 Codex 开发协作能力描述成对外生产能力。

#### Scenario: 后端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Backend.md`
- **THEN** 文档 MUST 说明内部试用支持的身份来源、真实 GitHub 工具显式启用、长期记忆观测和 smoke 门禁
- **AND** 文档 MUST 声明未实现完整 JWT/OIDC、租户体系、完整 MCP runtime 或运行时 sub-agent

#### Scenario: 前端 SDD 描述内部试用边界

- **WHEN** 读者查看 `docs/SDD-Frontend.md`
- **THEN** 文档 MUST 说明前端只展示后端返回的状态、限制和 trace
- **AND** 文档 MUST 声明前端不直接调用 GitHub、MCP 服务、用户中心或长期记忆写接口
