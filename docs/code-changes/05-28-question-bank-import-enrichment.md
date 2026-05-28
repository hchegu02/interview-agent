# 05-28 Question Bank Import Enrichment

## 背景

本地题库导入可能只有题干，没有 `skill_category`、`tags`、`difficulty`、`expected_points`、`rubric`、`sample_answer`、`follow_up_hints` 等 RAG 和面试评分需要的结构化字段。

## 变更

- 本地题库导入在解析后、暂存前增加 LLM 元数据补全。
- 只补缺失字段，不覆盖用户上传时已经提供的字段。
- `id` 和 `content` 始终以原始导入内容为准，避免 LLM 改写导致题目错配。
- 未配置 LLM 时保持旧行为：仍按默认 `skill_category=general`、`difficulty=3` 暂存。
- Mock LLM 增加题库元数据补全响应，保证单测和本地 mock 演示可复现。

## 验证

- 新增测试覆盖：
  - 只有题干时使用 LLM 补齐元数据。
  - 未配置 LLM 时保持旧默认兜底。

## 下一步

- 为批量补全增加分批大小和重试策略，避免大文件一次 prompt 过长。
- 在导入预览页显示 LLM 补全来源和字段差异，方便人工确认。
- 为真实模型补充更严格的 schema 校验，例如要求返回条数和原题一一对应。
