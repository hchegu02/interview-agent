// Package graph 是项目自研的轻量 DAG / 决策图执行框架。
//
// 设计目标：
//  1. 支持线性边 / 并发 fan-out / 条件分支（Router）/ 循环回边
//  2. 节点保持纯函数语义，副作用（SSE 推送、审计落库）通过 Callback 注入
//  3. 装饰器模式叠加 Retry / Timeout / Schema 等横切逻辑
//  4. 全局步数预算（maxSteps）兜底循环，配合 WorkingMemory 业务预算双保险
//
// 为什么自研而不引 Eino：
//   工程亮点（拓扑分析、errgroup、装饰器链、循环兜底）全在自己代码里，
//   面试时每一段都能讲；Eino 替代实现 ~50 行，但工程深度被框架吞掉。
//
// 文件分布：
//   - node.go        : 类型 + 错误集中定义
//   - graph.go       : Graph / Runnable / frontier 执行器
//   - decorators.go  : Retry / Timeout / Compose
package graph

import (
	"context"
	"errors"

	"interview-agent/internal/domain"
)

// NodeFunc 是节点的统一签名。
//
// 为什么不用泛型 In/Out：
//   - Agent 流程里所有节点都读写共享的 Session 聚合根
//   - 强类型 In/Out 会让"节点 A 写到 Session.X，节点 B 读 Session.X" 这种
//     跨节点状态变得很别扭
//   - 简化 Graph 实现，~200 行 vs ~600 行
//
// 代价：节点作者负责不并发写到同一字段。并发节点的字段写入必须 disjoint。
// 单测里 -race 会捕获违规。
type NodeFunc func(ctx context.Context, sess *domain.Session) error

// Router 是条件分支节点的下游决策函数。
//
// 在节点 fn 执行完后被调用，根据 Session 当前状态返回下一节点名。
// 返回 EndNode 终止图执行。
//
// 为什么 Router 是独立函数而非节点内部 if-else：
//   - 让"业务计算"和"流程路由"分离，节点函数保持纯
//   - Graph 可以静态分析 router 出度（虽然 Go 没法在编译期算）
//   - 单测时可以独立测试 router 逻辑，不依赖节点
type Router func(sess *domain.Session) string

// EndNode 是图执行终止的哨兵节点名。
// Router 返回此值表示当前路径结束。
const EndNode = "__END__"

// Callback 是节点生命周期钩子。
//
// 关键设计：节点函数不应该自己写 SSE / 落库 / 打指标，
// 这些副作用全部走 Callback。好处：
//   - 节点单测无需 mock 这些 IO
//   - Stage 5 SSE 推送 / Stage 4 token 计数都是各自一个 Callback 实现，
//     不污染节点代码
type Callback interface {
	OnNodeStart(ctx context.Context, node string, sess *domain.Session)
	OnNodeEnd(ctx context.Context, node string, sess *domain.Session)
	OnNodeError(ctx context.Context, node string, sess *domain.Session, err error)
}

// 错误集中声明
var (
	// ErrPermanent 标记不可重试错误。Retry 装饰器看到此错误立即返回。
	// 节点用 fmt.Errorf("...: %w", graph.ErrPermanent) 包裹。
	ErrPermanent = errors.New("graph: permanent error")

	// ErrMaxStepsExceeded 全局步数预算耗尽（保护无限循环）
	ErrMaxStepsExceeded = errors.New("graph: max steps exceeded")

	// ErrNodeNotFound 边/router 引用了未注册的节点
	ErrNodeNotFound = errors.New("graph: node not found")

	// ErrInvalidConfig 编译时校验失败（无入口、节点冲突等）
	ErrInvalidConfig = errors.New("graph: invalid configuration")

	// ErrSuspended 节点主动暂停图执行，等待外部输入（用户答题 / 人工干预）。
	//
	// 与 ErrPermanent 的区别：
	//   - ErrPermanent：流程出错，整图终止
	//   - ErrSuspended：流程正常，只是阶段性等用户
	//
	// 执行器看到此错误后：
	//   - 把当前节点名写到 sess.CurrentNode（断点）
	//   - Invoke 正常返回 nil，不算错误
	//   - 调用方根据 sess.CurrentNode != "" 判断是否需要 Resume
	//
	// 用户输入到位后调 Runnable.Resume(ctx, sess)，
	// 从 CurrentNode 的下游边继续推进。
	//
	// 节点可以用 fmt.Errorf("waiting for answer: %w", graph.ErrSuspended) 携带原因，
	// 上层用 errors.Is 检查。
	ErrSuspended = errors.New("graph: suspended waiting for external input")
)
