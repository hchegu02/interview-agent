# 06-08 questionbank import flex json

## 1. 变更概述

真实 LLM 生成题库 JSON 时，常把等价字段输出成非严格形态，例如 `difficulty` 输出为字符串、`rubric` 输出为数组或字符串、`tags` / `expected_points` / `follow_up_hints` 输出为逗号或分号分隔字符串。原导入解析只按 `Item` 严格反序列化，导致整批文档导入失败。

本次在题库 JSON 导入解析层增加有限容错：严格解析失败后，对上述字段做兼容转换，再进入后续校验、暂存和 review 流程。正式 `Item` 结构、数据库 schema、HTTP 响应结构不变。

## 2. 变更文件

- `internal/questionbank/imports_parse.go`
  - 增加 flexible JSON 解析 fallback。
  - 支持 `difficulty` string -> int。
  - 支持 `rubric` map / string array / string。
  - 支持 `tags`、`expected_points`、`role_tags`、`follow_up_hints` 的 string array 或分隔字符串。
  - `splitImportList` 增加中文分号 `；`。
- `internal/questionbank/imports_test.go`
  - 增加真实 LLM schema 漂移回归测试。

## 3. 函数级说明

- `parseJSONItems(raw []byte) ([]Item, error)`
  - 行为变化：原先严格解析失败直接返回错误；现在在 wrapped JSON 严格解析失败后尝试 `parseFlexibleJSONItems`。
  - 输入：题库 JSON 原始字节。
  - 输出：标准 `[]Item`。
  - 错误处理：兼容解析也失败时同时保留严格解析错误和兼容解析错误，方便定位字段级 schema 漂移。

- `parseFlexibleJSONItems(raw []byte) ([]Item, error)`
  - 新增函数。
  - 作用：接受顶层数组或 `{items:[...]}` 两种 JSON 形态，并逐项调用 flexible item 解析。

- `parseFlexibleJSONItem(raw json.RawMessage) (Item, error)`
  - 新增函数。
  - 作用：把容易漂移的字段从原 JSON 中取出单独解析，其余字段仍交给 `json.Unmarshal` 到 `Item`。
  - 副作用：无。

- `parseFlexibleDifficulty(raw json.RawMessage) (int, error)`
  - 新增函数。
  - 作用：支持数字或数字字符串难度。

- `parseFlexibleRubric(raw json.RawMessage) (map[string]string, error)`
  - 新增函数。
  - 作用：支持 map、字符串数组、单字符串三种 rubric。
  - 转换规则：数组转 `point_1`、`point_2`；字符串转 `general`。

- `parseFlexibleStringList(raw json.RawMessage) ([]string, error)`
  - 新增函数。
  - 作用：支持字符串数组或分隔字符串。
  - 行为变化：字段存在但既不是字符串数组也不是字符串时返回错误，避免静默吞掉坏字段。

- `compactImportStrings(list []string) []string`
  - 新增函数。
  - 作用：清理空字符串和首尾空白。

- `splitImportList(raw string) []string`
  - 行为变化：新增中文分号 `；` 作为分隔符。

## 4. 调用链

- 文档导入：
  - `ImportService.ImportDocument`
  - `processDocument`
  - `generateItems`
  - `validateItemsJSON`
  - `parseQuestionBankItems("generated.json", raw)`
  - `parseJSONItems`
  - `parseFlexibleJSONItems`，仅严格解析失败时进入。

- 本地题库 JSON 导入：
  - `ImportService.ImportLocalQuestionBank`
  - `processLocalQuestionBank`
  - `parseQuestionBankItems`
  - `parseJSONItems`
  - `parseFlexibleJSONItems`，仅严格解析失败时进入。

## 5. 数据流

LLM 或上传文件产生 JSON 字节，解析层先尝试严格 `Item` 结构。如果字段类型漂移，兼容解析把字段归一化为正式 `Item` 类型，再交给现有 normalize、quality gate、review 和 commit 流程。`tags`、`expected_points`、`role_tags`、`follow_up_hints` 只接受字符串数组或分隔字符串；对象、数字等不支持形态会返回解析错误，不再被当作空列表。

## 6. 依赖与副作用

- 无新增依赖。
- 无数据库结构变更。
- 无 HTTP API 字段变更。
- 影响范围限于题库导入 JSON 解析。

## 7. 测试

已执行：

```powershell
$env:GOCACHE="D:\Documents\New project\interview-agent\tmp\gocache"; go test ./internal/questionbank -run TestParseQuestionBankItemsToleratesLLMScalarDrift -count=1
$env:GOCACHE="D:\Documents\New project\interview-agent\tmp\gocache"; go test ./internal/questionbank -run "TestParseQuestionBankItems(ToleratesRubricArray|RejectsUnsupportedFlexibleStringList)" -count=1
$env:GOCACHE="D:\Documents\New project\interview-agent\tmp\gocache"; go test ./internal/questionbank -count=1
go test ./...
```

结果均通过。

真实导入验证：

- `负载均衡面试题.md` -> `imp-6a8464058cb5`，ready，9 valid。
- `Go语言面试题.md` -> `imp-730c3fbb40d3`，ready，35 valid。
- `Redis面试题.md` -> `imp-e3632d6dd240`，ready，20 valid。
- `MySQL面试题.md` -> `imp-d4c6882e46cf`，ready，20 valid。
- `分布式面试题.md` -> `imp-cbdfadcb3d99`，ready，19 valid。

## 8. 风险

- 兼容解析会接受更多 LLM 输出形态，但仍只归一化到既有 `Item` 字段，不绕过后续质量门禁。
- `rubric` 数组转 `point_N` 的 key 是生成的，不含业务语义；后续如需要更精细评分维度，应在生成 prompt 或 schema 层约束。
- 本次只是暂存导入，未自动 commit 到正式题库；仍需 review 后提交。
