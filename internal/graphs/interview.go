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
	nodeAnalyzeProfile  = "analyze_profile"
	nodeRetrieveRAG     = "retrieve_rag"
	nodeCritic          = "critic"
	nodeProbeEval       = "probe_eval"
	nodeReflectionCheck = "reflection_check"
)

// Deps contains the external services needed by the interview graph.
type Deps struct {
	Model              llm.ChatModel
	Embedder           embedding.Embedder
	Retriever          retriever.Retriever
	Callbacks          []graph.Callback
	CheckpointRecorder graph.CheckpointRecorder
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
		AddNode(nodeAnalyzeProfile, nodes.NewAnalyzeProfileNode()).
		AddNodeSpec(graph.NodeSpec{
			Name:   nodeRetrieveRAG,
			Fn:     nodes.NewRetrieveRAGNode(deps.Embedder, deps.Retriever, nodes.RetrieveRAGOptions{}),
			Writes: []string{graph.WriteCandidatePool, graph.WriteRetrievalTrace, graph.WriteWorkingMemory},
		}).
		AddNodeSpec(graph.NodeSpec{
			Name: nodes.NodePickNext,
			Fn:   nodes.NewPickNextNode(deps.Model, nodes.PickNextOptions{}),
			Writes: []string{
				graph.WritePendingDecision,
				graph.WriteRounds,
				graph.WriteWorkingMemory,
				graph.WriteSuspension,
			},
		}).
		AddNodeSpec(graph.NodeSpec{
			Name:   nodes.NodeEvaluate,
			Fn:     nodes.NewEvaluateNode(deps.Model, nodes.EvaluateOptions{}),
			Writes: []string{graph.WritePendingDecision, graph.WriteCurrentEvaluation, graph.WriteWorkingMemory},
		}).
		AddNode(nodeCritic, nodes.NewCriticNode(deps.Model, nodes.CriticOptions{})).
		AddNode(nodes.NodeRefine, nodes.NewRefineNode(deps.Model, nodes.RefineOptions{})).
		AddNode(nodes.NodeProbeAsk, nodes.NewProbeAskNode(deps.Model, nodes.ProbeAskOptions{})).
		AddNode(nodeProbeEval, nodes.NewProbeEvalNode(deps.Model, nodes.ProbeEvalOptions{})).
		AddNode(nodes.NodeUpdateMemory, nodes.NewUpdateMemoryNode(nodes.UpdateMemoryOptions{})).
		AddNode(nodes.NodeUpdateDifficulty, nodes.NewUpdateDifficultyNode(nodes.UpdateDifficultyOptions{})).
		AddNode(nodeReflectionCheck, nodes.NewReflectionCheckNode(deps.Model, nodes.ReflectionCheckOptions{})).
		AddNodeSpec(graph.NodeSpec{
			Name:   nodes.NodeReport,
			Fn:     nodes.NewReportNode(),
			Writes: []string{graph.WritePendingDecision, graph.WriteReport, graph.WriteStatus, graph.WriteWorkingMemory},
		}).
		Entry(nodeParseJD).
		AddEdge(nodeParseJD, nodeParseResume).
		AddEdge(nodeParseResume, nodeGapAnalyze).
		AddEdge(nodeGapAnalyze, nodeAnalyzeProfile).
		AddEdge(nodeAnalyzeProfile, nodeRetrieveRAG).
		AddEdge(nodeRetrieveRAG, nodes.NodePickNext).
		AddBranch(nodes.NodePickNext, nodes.RouteAfterPickNext).
		AddEdge(nodes.NodeEvaluate, nodeCritic).
		AddBranch(nodeCritic, nodes.RouteAfterCritic).
		AddBranch(nodes.NodeRefine, nodes.RouteAfterRefine).
		AddEdge(nodes.NodeProbeAsk, nodeProbeEval).
		AddBranch(nodeProbeEval, nodes.RouteAfterProbeEval).
		AddEdge(nodes.NodeUpdateMemory, nodes.NodeUpdateDifficulty).
		AddEdge(nodes.NodeUpdateDifficulty, nodeReflectionCheck).
		AddBranch(nodeReflectionCheck, nodes.RouteAfterReflection).
		AddEdge(nodes.NodeReport, graph.EndNode)

	if len(deps.Callbacks) > 0 {
		g.WithCallbacks(deps.Callbacks...)
	}
	if deps.CheckpointRecorder != nil {
		g.WithCheckpointRecorder(deps.CheckpointRecorder)
	}
	return g.Compile()
}
