// Package graphs assembles runnable interview workflows from graph nodes.
package graphs

import (
	"fmt"

	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/nodes"
	"interview-agent/internal/retriever"
)

const (
	nodeParseJD         = "parse_jd"
	nodeParseResume     = "parse_resume"
	nodeGapAnalyze      = "gap_analyze"
	nodeRetrieveRAG     = "retrieve_rag"
	nodeCritic          = "critic"
	nodeProbeEval       = "probe_eval"
	nodeReflectionCheck = "reflection_check"
)

// Deps contains the external services needed by the interview graph.
type Deps struct {
	Model     llm.ChatModel
	Embedder  embedding.Embedder
	Retriever retriever.Retriever
	Callbacks []graph.Callback
}

// BuildInterviewGraph wires setup nodes and the agent loop into one Runnable.
func BuildInterviewGraph(deps Deps) (*graph.Runnable, error) {
	if deps.Model == nil {
		return nil, fmt.Errorf("%w: model is required", graph.ErrInvalidConfig)
	}
	if deps.Embedder == nil {
		return nil, fmt.Errorf("%w: embedder is required", graph.ErrInvalidConfig)
	}
	if deps.Retriever == nil {
		return nil, fmt.Errorf("%w: retriever is required", graph.ErrInvalidConfig)
	}

	g := graph.New("interview").
		AddNode(nodeParseJD, nodes.NewParseJDNode(deps.Model)).
		AddNode(nodeParseResume, nodes.NewParseResumeNode(deps.Model)).
		AddNode(nodeGapAnalyze, nodes.NewGapAnalyzeNode(deps.Model)).
		AddNode(nodeRetrieveRAG, nodes.NewRetrieveRAGNode(deps.Embedder, deps.Retriever, nodes.RetrieveRAGOptions{})).
		AddNode(nodes.NodePickNext, nodes.NewPickNextNode(deps.Model, nodes.PickNextOptions{})).
		AddNode(nodes.NodeEvaluate, nodes.NewEvaluateNode(deps.Model, nodes.EvaluateOptions{})).
		AddNode(nodeCritic, nodes.NewCriticNode(deps.Model, nodes.CriticOptions{})).
		AddNode(nodes.NodeRefine, nodes.NewRefineNode(deps.Model, nodes.RefineOptions{})).
		AddNode(nodes.NodeProbeAsk, nodes.NewProbeAskNode(deps.Model, nodes.ProbeAskOptions{})).
		AddNode(nodeProbeEval, nodes.NewProbeEvalNode(deps.Model, nodes.ProbeEvalOptions{})).
		AddNode(nodes.NodeUpdateMemory, nodes.NewUpdateMemoryNode(nodes.UpdateMemoryOptions{})).
		AddNode(nodeReflectionCheck, nodes.NewReflectionCheckNode(deps.Model, nodes.ReflectionCheckOptions{})).
		AddNode(nodes.NodeReport, nodes.NewReportNode()).
		Entry(nodeParseJD).
		AddEdge(nodeParseJD, nodeParseResume).
		AddEdge(nodeParseResume, nodeGapAnalyze).
		AddEdge(nodeGapAnalyze, nodeRetrieveRAG).
		AddEdge(nodeRetrieveRAG, nodes.NodePickNext).
		AddBranch(nodes.NodePickNext, nodes.RouteAfterPickNext).
		AddEdge(nodes.NodeEvaluate, nodeCritic).
		AddBranch(nodeCritic, nodes.RouteAfterCritic).
		AddBranch(nodes.NodeRefine, nodes.RouteAfterRefine).
		AddEdge(nodes.NodeProbeAsk, nodeProbeEval).
		AddBranch(nodeProbeEval, nodes.RouteAfterProbeEval).
		AddEdge(nodes.NodeUpdateMemory, nodeReflectionCheck).
		AddBranch(nodeReflectionCheck, nodes.RouteAfterReflection).
		AddEdge(nodes.NodeReport, graph.EndNode)

	if len(deps.Callbacks) > 0 {
		g.WithCallbacks(deps.Callbacks...)
	}
	return g.Compile()
}
