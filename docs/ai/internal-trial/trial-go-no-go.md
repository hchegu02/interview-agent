# 内部试用 Go/No-Go 标准

本文定义 internal trial rollout loop 的 Go/No-Go 判断标准。它只用于内部试用推进，不代表生产发布批准。

## Enter Technical Trial

只有满足以下条件，才可以进入 technical trial：

- Core verification passes / 核心验证通过。
- 候选流程仍限定在内部试用范围内，未扩大到生产用户、生产 SLA 或生产发布承诺。
- 工具驱动请求必须保留一致、可审计的 `tool_trace`，并能解释工具调用、结果和最终回答之间的关系。
- mock 路径仍可用，真实工具异常时可以优先回滚 mock。
- 关键失败可以稳定复现，至少有可复跑的输入、会话记录或验证命令。

## Enter Business Trial

只有 technical trial 稳定后，才可以进入 business trial：

- technical trial 中发现的阻断问题已经关闭，且没有未解释的 core verification failure / 核心验证失败。
- 工具调用结果、`tool_trace` 和用户可见回答一致，没有把真实工具失败描述为成功。
- 身份边界行为符合预期；不得出现 Identity boundary accepts dev fallback unexpectedly / 身份边界意外接受开发 fallback。
- 业务试用仍是内部试用，不是生产发布，不引入新的生产依赖、生产数据范围或外部用户承诺。
- 真实工具链路失败时，默认先暂停推进并回滚 mock，而不是扩大真实工具覆盖面。

## Pause Rollout

出现以下任一 hard pause 条件，必须暂停 rollout：

- Core verification fails / 核心验证失败。
- `tool_trace` is missing or contradictory for tool-driven requests。
- Real tool failure is described as success / 真实工具失败被描述为成功。
- Identity boundary accepts dev fallback unexpectedly / 身份边界意外接受开发 fallback。
- Critical failures cannot be reproduced / 关键失败不可复现。

暂停后只允许做问题定位、复现、修复和验证，不允许继续扩大试用范围。

## Roll Back To Mock

真实工具问题优先回滚 mock。以下情况必须回滚或保持 mock 路径：

- 真实工具失败影响核心面试流程、评分、报告生成或验证门禁。
- 真实工具输出无法通过 `tool_trace` 解释，或者 trace 与最终回答矛盾。
- 真实工具失败被包装成成功，导致用户或评估者误判系统能力。
- 身份边界、权限边界或开发 fallback 行为不稳定。
- 关键失败不可复现，无法判断是工具、状态、配置还是提示链路问题。

回滚 mock 不是放弃试用，而是保持内部试用可控。恢复真实工具前，必须重新通过相关验证。

## Do Not Expand Scope

以下条件下禁止扩大范围：

- 任一 hard pause 条件仍未关闭。
- 当前阶段仍依赖未验证的真实工具能力。
- mock 回滚路径不可用或未验证。
- 新增范围会引入生产发布语义、生产用户、生产数据、生产 SLA 或不可逆外部副作用。
- 现有问题只能靠人工解释，不能靠代码、配置、测试、trace 或 runbook 复现和验证。

Scope expansion 必须服务于内部试用目标。不能用扩大范围掩盖核心验证失败、真实工具不稳定或身份边界错误。
