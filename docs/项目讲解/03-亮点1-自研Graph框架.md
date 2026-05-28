# 亮点 1：自研轻量 Graph 框架（frontier-based DAG 执行引擎）

> 代码位置：`internal/graph/`（共 4 个文件，~500 行）
> 关键文件：`node.go`、`graph.go`、`decorators.go`、`graph_test.go`
> 面试官追问命中率：★★★★★

---

## TL;DR（30 秒讲清楚版）

> 项目自研了一个轻量决策图框架，**不是 Eino 套壳**。核心抽象是 `NodeFunc(ctx, *Session) error` + 纯函数 `Router`，用 **frontier-based 执行模型** 支持线性边 / 并发 fan-out / 条件分支 / 循环回边，通过 `ErrSuspended` 哨兵实现节点挂起 / `Resume` 恢复（这是支持"AI 出题 → 等用户答 → AI 评估"这种异步交互的关键），用 `ErrPermanent` 显式区分可重试 / 不可重试错误，全局 `MaxSteps` 兜底死循环。整个框架 ~500 行，强制架构约束写进 `CLAUDE.md`。

---

## 一、为什么自研？（这是面试官第一个会问的）

### 看似简单的需求，找不到完全合适的开源库

我的需求清单：

| 需求 | Eino | 通用 DAG 库（如 dag-engine）| 工作流引擎（如 Temporal）|
|---|---|---|---|
| Go 原生、无外部依赖 | ✅ | ✅ | ❌ 需要 Server |
| 节点挂起 / 等待外部输入 / 恢复 | ⚠️ 复杂 Callback | ❌ | ✅ |
| Router 纯函数强约束 | ❌ Router 可副作用 | ❌ | ❌ |
| 错误分类（永久 / 临时 / 挂起）| ⚠️ 自己包 | ❌ | ⚠️ 框架化 |
| Fan-out 并发 + Fan-in 汇聚 | ✅ | ✅ | ✅ |
| 全局步数预算防死循环 | ❌ | ❌ | ✅ |
| 单元测试零外部依赖 | ✅ | ✅ | ❌ |
| 框架边界小 / 一辈子能维护 | ❌ 重 | ⚠️ | ❌ |

### 真正的取舍

> **"框架不是越大越好，强约束 + 小代码量更易长期维护。"**

自研只用 ~500 行：
- `node.go` 99 行：类型定义 + 错误集中声明
- `graph.go` 295 行：Graph / Runnable / frontier 执行器
- `decorators.go` ~150 行：Retry / Timeout / Compose 装饰器
- `graph_test.go` ~400 行：完整 happy/error/suspend 路径测试

代码注释里我留了原话（`node.go` line 9-11）：

> **为什么自研而不引 Eino**：工程亮点（拓扑分析、errgroup、装饰器链、循环兜底）全在自己代码里，面试时每一段都能讲；Eino 替代实现 ~50 行，但工程深度被框架吞掉。

---

## 二、核心抽象

### 2.1 节点签名（统一、强约束）

```go
// NodeFunc 是节点的统一签名。
type NodeFunc func(ctx context.Context, sess *domain.Session) error
```

**为什么不用泛型 In/Out**：

- Agent 流程里所有节点都读写共享的 Session 聚合根
- 强类型 In/Out 会让"节点 A 写到 `Session.X`，节点 B 读 `Session.X`"这种跨节点状态变得很别扭
- 简化 Graph 实现，~200 行 vs ~600 行

**代价**：节点作者负责不并发写到同一字段。并发节点的字段写入必须 **disjoint**。单测里 `-race` 会捕获违规。

### 2.2 Router（纯函数 + 强约束）

```go
// Router 是条件分支节点的下游决策函数。
type Router func(sess *domain.Session) string
```

**架构规则写进 `CLAUDE.md`：**

> Router 必须是纯函数，**禁止副作用**。所有写 `PendingDecision`、`Notes` 的动作必须在节点里完成。

**为什么这样设计**：

1. **可测试** —— router 单测无需 mock 任何依赖
2. **可重放** —— 同样的 Session 状态永远走同一条路径
3. **职责分离** —— "业务计算"（节点）和"流程路由"（router）完全解耦

```go
// 示例：好的 router（纯函数）
func RouteAfterCritic(sess *domain.Session) string {
    round := sess.CurrentRound()
    if round.Critic == nil {
        return nodes.NodeUpdateMemory
    }
    if round.Critic.NeedsRefine {
        return nodes.NodeRefine
    }
    if round.Critic.HasProbeSignal {
        return nodes.NodeProbeAsk
    }
    return nodes.NodeUpdateMemory
}

// 编译期检查（防止误改签名）
var _ graph.Router = RouteAfterCritic
```

### 2.3 错误集中分类

```go
var (
    ErrPermanent        = errors.New("graph: permanent error")
    ErrMaxStepsExceeded = errors.New("graph: max steps exceeded")
    ErrNodeNotFound     = errors.New("graph: node not found")
    ErrInvalidConfig    = errors.New("graph: invalid configuration")
    ErrSuspended        = errors.New("graph: suspended waiting for external input")
)
```

每种错误的语义和影响：

| 错误 | 来源 | runtime 行为 | 节点作者怎么用 |
|---|---|---|---|
| `ErrPermanent` | 配置错 / 参数错 | 立即终止整图 | `fmt.Errorf("bad config: %w", graph.ErrPermanent)` |
| `ErrSuspended` | 节点主动暂停等待用户 | **不算错误**，正常返回 nil，记 `CurrentNode` | `fmt.Errorf("waiting for answer: %w", graph.ErrSuspended)` |
| `ErrMaxStepsExceeded` | 全局步数兜底 | 终止图，记 last frontier | runtime 抛 |
| 普通 error | 节点业务逻辑 | 继续返回给调用方，调用方决定降级 | 直接 `return err` |

---

## 三、Frontier-based 执行模型（核心算法）

### 3.1 为什么是 frontier 而不是事件驱动 / 协程长跑？

> 维护一个"待执行节点集合" frontier，每一轮把 frontier 里的所有节点用 errgroup 并发执行，等全部完成后根据 edges/routers 算出下一轮 frontier，直到 frontier 为空或步数耗尽。

**对比其他执行模型**：

| 执行模型 | 优点 | 缺点 | 适用场景 |
|---|---|---|---|
| **Frontier-based**（我的方案）| 自然支持 fan-out + fan-in，与 errgroup 契合，状态简单（断点只记当前 frontier）| 每轮要算 next frontier | 多节点并行 + 需要持久化的图 |
| **事件驱动** | 实时响应、低延迟 | 状态散落各处，难以 trace | 实时流处理 |
| **协程长跑**（每节点一个 goroutine + channel）| 编程模型自然 | goroutine 泄漏风险、断点复杂 | 长生命周期流 pipeline |

### 3.2 完整执行流程

```go
// run 是 Invoke / Resume 共享的执行内核。
func (r *Runnable) run(ctx context.Context, sess *domain.Session, frontier []string) error {
    g := r.g
    steps := 0

    for len(frontier) > 0 {
        steps++
        if steps > g.maxSteps {
            return fmt.Errorf("%w: %d steps (graph=%s, last frontier=%v)",
                ErrMaxStepsExceeded, g.maxSteps, g.name, frontier)
        }

        if err := r.executeBatch(ctx, sess, frontier); err != nil {
            if errors.Is(err, ErrSuspended) {
                // 暂停：节点已把自己名字写到 sess.CurrentNode
                return nil
            }
            return err
        }

        frontier = r.nextFrontier(sess, frontier)
    }
    return nil
}
```

### 3.3 并发执行细节

```go
func (r *Runnable) executeBatch(ctx context.Context, sess *domain.Session, nodes []string) error {
    if len(nodes) == 1 {
        return r.executeNode(ctx, sess, nodes[0])   // 单节点跳过 errgroup 开销
    }

    eg, ectx := errgroup.WithContext(ctx)            // 多节点用 errgroup 并发
    for _, name := range nodes {
        name := name
        eg.Go(func() error {
            return r.executeNode(ectx, sess, name)
        })
    }
    return eg.Wait()
}
```

**优化点**：

- **单节点 fast path**：避免不必要的 goroutine + channel 开销
- **errgroup.WithContext**：任一节点失败立即 cancel 其他兄弟节点
- **闭包变量捕获**：经典 Go for-range 陷阱，必须显式 `name := name`

### 3.4 Fan-out + Fan-in 的统一实现

```go
func (r *Runnable) nextFrontier(sess *domain.Session, prev []string) []string {
    nextSet := make(map[string]struct{})  // map dedup 处理 fan-in

    for _, name := range prev {
        if router, ok := g.routers[name]; ok {
            target := router(sess)
            if target != EndNode && target != "" {
                nextSet[target] = struct{}{}
            }
            continue
        }
        for _, to := range g.edges[name] {  // 多条 edge = fan-out 并发
            if to != EndNode {
                nextSet[to] = struct{}{}
            }
        }
    }

    out := make([]string, 0, len(nextSet))
    for n := range nextSet {
        out = append(out, n)
    }
    sort.Strings(out)  // 确定性顺序，便于 -race 之外的可复现测试
    return out
}
```

**关键设计**：

1. **Fan-out**：同一个 `from` 配多条 edge → 下一轮 frontier 多节点 → errgroup 并发
2. **Fan-in**：多条 edge 汇聚到同一节点 → map dedup 保证只执行一次
3. **排序**：返回前 `sort.Strings(out)`，让 frontier 顺序确定，便于 race 之外的可复现性测试

---

## 四、Suspend/Resume 机制（最难、最关键）

### 4.1 为什么需要

Agent 流程：

```
pick_next  →  ❓ 等用户答  →  evaluate  →  critic  →  ...
```

中间的"等用户答"可能是几秒到几小时（用户可能去倒杯水回来再答），**不能让 goroutine 一直挂着等**。所以需要：

- `pick_next` 节点出完题 → return `ErrSuspended`
- runtime 把当前节点名记到 `sess.CurrentNode`，**Invoke 正常返回 nil**
- HTTP 层把 session 状态持久化
- 用户答到时，HTTP 层把 answer 写到 session → 调 `Runnable.Resume(ctx, sess)`
- Runtime 从 `sess.CurrentNode` 的下游 edges/router 算出 next frontier 继续

### 4.2 节点侧的写法

```go
// pick_next 节点示例
func PickNext(ctx context.Context, sess *domain.Session) error {
    // ... 用 LLM 或规则挑下一题 ...
    sess.WorkingMemory.CurrentQuestion = q

    // 等用户答题：suspend
    return fmt.Errorf("waiting for answer: %w", graph.ErrSuspended)
}
```

### 4.3 Runtime 侧的处理

```go
// Invoke 内部
if err := r.executeBatch(ctx, sess, frontier); err != nil {
    if errors.Is(err, ErrSuspended) {
        // 暂停：节点已把自己名字写到 sess.CurrentNode（在 executeNode 里）
        return nil   // 返回 nil 让 HTTP 层正常响应
    }
    return err
}
```

### 4.4 外部调用者侧

```go
// HTTP 层 answer 接口
func (s *InterviewService) Answer(ctx context.Context, req answerInterviewRequest) (*domain.Session, error) {
    sess, _ := s.store.Get(ctx, req.SessionID)

    // 把用户答案写到当前 round
    fillPendingAnswer(sess, req.Answer)

    // 从 CurrentNode 的下游继续推进
    if err := s.runner.Resume(ctx, sess); err != nil {
        return nil, err
    }

    return sess, nil
}
```

### 4.5 Resume 的实现细节

```go
func (r *Runnable) Resume(ctx context.Context, sess *domain.Session) error {
    sess.MigrateLegacyState()                   // 兼容老 session
    if sess.CurrentNode == "" {
        return fmt.Errorf("%w: resume requires sess.CurrentNode", ErrInvalidConfig)
    }
    if _, ok := r.g.nodes[sess.CurrentNode]; !ok {
        return fmt.Errorf("%w: suspended node %q not in graph", ErrNodeNotFound, sess.CurrentNode)
    }
    // 关键：Resume 不会重跑 sess.CurrentNode
    // 假设该节点已经完成"暂停前"的工作，直接从该节点的 edges / router 算下一轮 frontier
    next := r.nextFrontier(sess, []string{sess.CurrentNode})
    if len(next) == 0 {
        return nil
    }
    return r.run(ctx, sess, next)
}
```

**最容易出 bug 的点**：

> **Resume 不重跑 CurrentNode** —— 这是契约。节点作者必须保证"return ErrSuspended 之前，所有写 session 的操作已经完成"。否则 Resume 后状态会丢一截。

---

## 五、装饰器模式（Retry / Timeout）

### 5.1 装饰器链

```go
// 实际使用：
node := graph.WithRetry(
    graph.WithTimeout(
        nodes.PickNext,
        10*time.Second,
    ),
    3,                          // 最多重试 3 次
    100*time.Millisecond,       // 初始 backoff
)
```

### 5.2 关键细节

```go
func WithRetry(fn NodeFunc, maxAttempts int, baseBackoff time.Duration) NodeFunc {
    return func(ctx context.Context, sess *domain.Session) error {
        var err error
        for attempt := 1; attempt <= maxAttempts; attempt++ {
            err = fn(ctx, sess)
            if err == nil {
                return nil
            }

            // 关键：ErrPermanent / ErrSuspended 不重试
            if errors.Is(err, ErrPermanent) || errors.Is(err, ErrSuspended) {
                return err
            }

            // 指数退避
            backoff := time.Duration(attempt) * baseBackoff
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(backoff):
            }
        }
        return err
    }
}
```

**设计要点**：

1. **`ErrPermanent` 不重试** —— 配置错重试 100 次也不会好
2. **`ErrSuspended` 不重试** —— 这是正常暂停，不是错误
3. **ctx 取消时立刻退出** —— 不能 sleep 等 backoff 等到天荒地老
4. **指数退避** —— 避免雷击效应

---

## 六、Compile 时校验（防止启动后才暴露的 bug）

```go
func (g *Graph) Compile() (*Runnable, error) {
    if g.entry == "" {
        return nil, fmt.Errorf("%w: entry not set", ErrInvalidConfig)
    }
    if _, ok := g.nodes[g.entry]; !ok {
        return nil, fmt.Errorf("%w: entry node %q not registered", ErrInvalidConfig, g.entry)
    }

    // 所有边引用的节点必须已注册（EndNode 除外）
    for from, tos := range g.edges {
        if _, ok := g.nodes[from]; !ok {
            return nil, fmt.Errorf("%w: edge from undefined node %q", ErrNodeNotFound, from)
        }
        for _, to := range tos {
            if to == EndNode { continue }
            if _, ok := g.nodes[to]; !ok {
                return nil, fmt.Errorf("%w: edge to undefined node %q (from %q)", ErrNodeNotFound, to, from)
            }
        }
    }

    // Router 必须注册在已存在的节点上
    for from := range g.routers {
        if _, ok := g.nodes[from]; !ok {
            return nil, fmt.Errorf("%w: router on undefined node %q", ErrNodeNotFound, from)
        }
        // 静态边和 router 不能同时存在
        if _, ok := g.edges[from]; ok {
            return nil, fmt.Errorf("%w: node %q has both static edges and router", ErrInvalidConfig, from)
        }
    }

    return &Runnable{g: g}, nil
}
```

**取舍**：

> 启动时校验比运行时报错好 100 倍。配置错应该在 server boot 阶段就 fail，不应该等用户访问到第 17 个节点才报"node not found"。

---

## 七、Callback 钩子（节点不写 IO）

### 7.1 设计目标

```go
type Callback interface {
    OnNodeStart(ctx context.Context, node string, sess *domain.Session)
    OnNodeEnd(ctx context.Context, node string, sess *domain.Session)
    OnNodeError(ctx context.Context, node string, sess *domain.Session, err error)
}
```

> **节点函数不应该自己写 SSE / 落库 / 打指标**，这些副作用全部走 Callback。

**好处**：

- 节点单测无需 mock IO
- SSE 推送 / token 计数 / OTel trace 各自一个 Callback 实现，**不污染节点代码**
- 多个 Callback 按注册顺序执行，**可组合**

### 7.2 实际使用

```go
// observability/recording_callback.go 实现 Callback
type RecordingCallback struct {
    records []NodeRecord
}

func (r *RecordingCallback) OnNodeStart(ctx context.Context, node string, sess *domain.Session) {
    r.records = append(r.records, NodeRecord{
        Node:    node,
        StartAt: time.Now(),
    })
}

// 注册到图
graph.New("interview").
    AddNode("parse_jd", parseJD).
    // ...
    WithCallbacks(recordingCB, sseCB, metricsCB).
    Compile()
```

---

## 八、完整使用示例

```go
// 业务侧组装两层子图（internal/graphs/interview.go）

func BuildInterviewGraph(nodes *Nodes) (*graph.Runnable, error) {
    g := graph.New("interview").
        // setup 子图（线性）
        AddNode("parse_jd",      nodes.ParseJD).
        AddNode("parse_resume",  nodes.ParseResume).
        AddNode("gap_analyze",   nodes.GapAnalyze).
        AddNode("retrieve_rag",  nodes.RetrieveRAG).

        // agent loop 子图（含 suspend + 循环）
        AddNode("pick_next",        nodes.PickNext).        // suspend 在这
        AddNode("evaluate",         nodes.Evaluate).
        AddNode("critic",           nodes.Critic).
        AddNode("refine",           nodes.Refine).
        AddNode("probe_ask",        nodes.ProbeAsk).        // suspend 在这
        AddNode("probe_eval",       nodes.ProbeEval).
        AddNode("update_memory",    nodes.UpdateMemory).
        AddNode("reflection_check", nodes.ReflectionCheck).

        // report
        AddNode("report", nodes.Report).

        Entry("parse_jd").
        AddEdge("parse_jd",     "parse_resume").
        AddEdge("parse_resume", "gap_analyze").
        AddEdge("gap_analyze",  "retrieve_rag").
        AddEdge("retrieve_rag", "pick_next").

        // pick_next 后 suspend，等 answer 后从 evaluate 继续
        AddEdge("pick_next",  "evaluate").
        AddEdge("evaluate",   "critic").

        AddBranch("critic",         routers.RouteAfterCritic).
        AddBranch("refine",         routers.RouteAfterRefine).
        AddBranch("probe_eval",     routers.RouteAfterProbeEval).
        AddEdge("update_memory",    "reflection_check").
        AddBranch("reflection_check", routers.RouteAfterReflection).
        AddEdge("report", graph.EndNode).

        MaxSteps(300)

    return g.Compile()
}
```

---

## 九、可能的追问与回答

### Q：为什么 Graph 没用泛型？

**A**：泛型 NodeFunc 会变成 `NodeFunc[In, Out]`，但是节点之间需要传共享状态，最后还是要回到 `Session`。强引入泛型只是"看起来现代"，实际上让代码更复杂，编译时间也更长。**这是 Go 哲学："Make it simple, not clever."**

### Q：Frontier-based 有没有性能问题？

**A**：每轮算 next frontier 是 O(节点数)，对 9 个节点来说常数级开销。**真正的瓶颈是 LLM 调用（3-5 秒）和 PG 查询（几十毫秒）**，graph 内部开销可以忽略。

### Q：Suspend 时怎么持久化 session？

**A**：Suspend 本身只把节点名写到 `sess.CurrentNode`，**持久化是 HTTP 层的责任**。HTTP 层在 `Invoke` 返回 nil 后看到 `sess.CurrentNode != ""` 就把 session 序列化写到 SessionStore（内存 / PG / Redis）。这是经典的 **"框架不管持久化，框架管语义"** 设计。

### Q：MaxSteps 默认 200 怎么算的？

**A**：每轮面试期望 5-10 题，每题平均经过 6 个节点（pick → eval → critic → refine? → probe* → update → reflect），加上 setup 4 个节点 + report 1 个节点 = **理论最大 ~75 节点访问**。200 是 3 倍冗余，留足 buffer 但能在异常死循环时及时报错。**业务循环用 `WorkingMemory.ProbeBudget` / `ReflectBudget` 做精确预算，MaxSteps 只是兜底**。

### Q：和 LangGraph 比怎么样？

**A**：LangGraph 是 Python LLM 框架，Go 生态没有等价。我的实现思路和 LangGraph 类似（声明式图 + 状态机），但 LangGraph 重在 LLM 编排，我重在分布式可靠性（suspend/resume + 强约束 + 错误分类）。**两者关注点不同，不冲突**。

---

## 十、可量化产出

- **代码量**：~500 行（包含完整测试）
- **测试覆盖**：节点测试、集成测试、`-race` 测试全部通过
- **架构规则**：5 条写进 `CLAUDE.md`（包级约束、Router 纯函数、错误分类、Compile 校验、Callback 副作用隔离）
- **支撑业务**：9 节点 Agent 图 + 两层子图（setup + agent loop）

---

## 📎 关联代码位置（面试时打开给面试官看）

| 设计点 | 代码位置 |
|---|---|
| NodeFunc / Router / Callback 定义 | `internal/graph/node.go` |
| Graph builder + Compile | `internal/graph/graph.go` line 14-128 |
| Frontier 执行内核 | `internal/graph/graph.go` line 181-218 |
| Suspend / Resume | `internal/graph/graph.go` line 165-179 |
| Retry / Timeout 装饰器 | `internal/graph/decorators.go` |
| Frontier 算法（fan-out/fan-in dedup）| `internal/graph/graph.go` line 268-294 |
| 业务图组装 | `internal/graphs/interview.go` |
