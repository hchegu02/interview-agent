## 1. 后端报告契约

- [x] 1.1 扩展报告领域模型，增加兼容的逐题复盘结构。
- [x] 1.2 从 `Session.Rounds` 构建主问题和已回答追问明细，保留题目、原始回答、评分和评分证据。
- [x] 1.3 明确总分聚合口径，未回答或未评分题不得计入有效总分。
- [x] 1.4 保持旧 JSON 响应兼容，新字段使用 `omitempty`。

## 2. 报告一致性验证

- [x] 2.1 增加报告一致性 verifier，检查缺题、缺原答案、缺逐题评分和总分不一致。
- [x] 2.2 将 verifier 接入 `cmd/agent-verify` 或 `cmd/internal-trial-smoke` 的内部试用门禁。
- [x] 2.3 增加通过和失败 fixture。

## 3. 前端报告页

- [x] 3.1 更新 `web/src/types.ts` 的报告逐题复盘类型。
- [x] 3.2 报告页优先展示 `report.round_reviews`，每题显示题目、原答案、单题评分、命中点、缺失点和建议。
- [x] 3.3 已回答追问显示为题目明细下的子项。
- [x] 3.4 将 Agent/Graph/检索 trace 等诊断信息移到报告后部或诊断区域。

## 4. 模拟 / 考试最小展示隔离

- [x] 4.1 考试模式隐藏题库选择入口和 Agent/Graph/事件调试状态。
- [x] 4.2 模拟模式保留训练辅助，但后端状态不得出现在题目前面。
- [x] 4.3 前端测试覆盖 practice/exam 展示差异。

## 5. 真实 JD 内部试用样例

- [x] 5.1 将 `D:/Downloads/云智研发公司 - 校园招聘 JD.md` 的岗位要求整理为内部试用样例或 fixture。
- [x] 5.2 不导入完整申请表单字段；招聘信息采集改造留到后续 change。

## 6. 验证和文档

- [x] 6.1 增加或更新 Go 测试。
- [x] 6.2 运行 `go test ./... -count=1`。
- [x] 6.3 运行 `npm --prefix web run test`。
- [x] 6.4 运行 `npm --prefix web run build`。
- [x] 6.5 运行相关 `agent-verify` / `internal-trial-smoke` 门禁。
- [x] 6.6 更新 `docs/code-changes/MM-DD-*.md`。
