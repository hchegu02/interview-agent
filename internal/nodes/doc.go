// Package nodes 定义所有 graph 节点的工厂函数。
//
// 每个文件实现一个或一组紧密相关的节点。节点签名都是
// `graph.NodeFunc = func(ctx, *Session) error`——节点工厂返回这个闭包。
//
// 为什么用工厂函数：
//   - 节点本身需要 LLM / Embedder / Retriever 等依赖，但 NodeFunc 签名
//     不允许传额外参数（保持 graph 框架简单）
//   - 工厂闭包 = "把依赖关进函数体里",一次注入,后续 Invoke 自动可用
//   - 单测时构造 mock 依赖、调用工厂、拿到节点函数,无需任何全局状态
//
// 阶段 2.5 提供 4 个 Setup 节点：
//   - parse_jd:      JD → JobProfile（LLM + schema 自纠正）
//   - parse_resume:  Resume → CandidateProfile（LLM + schema 自纠正）
//   - gap_analyze:   JD + Resume → GapReport（规则 + LLM 兜底）
//   - retrieve_rag:  GapReport → CandidatePool（Embedder + Retriever，含降级）
package nodes
