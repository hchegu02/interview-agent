# 05-29 Question Bank Import Diff Preview

## 背景

LLM 会补齐本地题库导入的结构化元数据。提交入库前，管理端需要看到哪些字段来自上传文件，哪些字段由 LLM 补齐或合并。

## 变更

- `ImportItem` 增加 `original_item`，保存上传解析后的原始题目。
- PG import store 使用已有 `raw_json` 列保存原始题目，不新增迁移。
- 导入详情接口返回 `original_item`。
- 前端导入预览展示关键字段差异：
  - 技能
  - 难度
  - 标签
  - 要点
  - Rubric
  - 参考答案
  - 追问
- 差异行标注来源：上传、LLM、合并。

## 验证

- 后端测试覆盖 enriched import 保留原始题目。
- 前端 typecheck 和 production build 通过。
