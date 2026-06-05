## 1. 设计文档

- [x] 1.1 阅读 README 和现有设计记录，确认当前架构事实。
- [x] 1.2 创建 `docs/SDD-Frontend.md`，覆盖页面结构、路由、草稿状态、API 调用、SSE 消费、报告展示、错误处理、前端验证和非目标章节。
- [x] 1.3 创建 `docs/SDD-Backend.md`，覆盖 HTTP API、Session、Agent Graph、RAG、LLM/rerank、存储、降级、可观测性、后端验证和非目标章节。
- [x] 1.4 确保两份 SDD 中的能力描述与真实仓库路径和命令一致，且不把后续 Codex sub-agent 开发方式写成当前运行时能力。

## 2. 验证

- [x] 2.1 检查两份 SDD 是否包含 spec 要求的全部主题。
- [x] 2.2 对关键路径和命令引用做轻量文本检查。
- [x] 2.3 查看 git diff，确保没有修改业务代码或生成产物。
