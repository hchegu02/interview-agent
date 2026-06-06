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
		AddNodeSpec(graph.PatchNode(
			nodeRetrieveRAG,
			[]string{graph.WriteCandidatePool, graph.WriteRetrievalTrace, graph.WriteWorkingMemory},
			nodes.NewRetrieveRAGPatchNode(deps.Embedder, deps.Retriever, nodes.RetrieveRAGOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodes.NodePickNext,
			[]string{
				graph.WritePendingDecision,
				graph.WriteRounds,
				graph.WriteWorkingMemory,
				graph.WriteSuspension,
			},
			nodes.NewPickNextPatchNode(deps.Model, nodes.PickNextOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodes.NodeEvaluate,
			[]string{graph.WritePendingDecision, graph.WriteCurrentEvaluation, graph.WriteWorkingMemory},
			nodes.NewEvaluatePatchNode(deps.Model, nodes.EvaluateOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodeCritic,
			[]string{graph.WriteCurrentCriticResult, graph.WriteWorkingMemory},
			nodes.NewCriticPatchNode(deps.Model, nodes.CriticOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodes.NodeRefine,
			[]string{graph.WriteCurrentRefinedEval, graph.WriteWorkingMemory},
			nodes.NewRefinePatchNode(deps.Model, nodes.RefineOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodes.NodeProbeAsk,
			[]string{graph.WriteRounds, graph.WriteWorkingMemory},
			nodes.NewProbeAskPatchNode(deps.Model, nodes.ProbeAskOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodeProbeEval,
			[]string{graph.WriteRounds, graph.WriteWorkingMemory},
			nodes.NewProbeEvalPatchNode(deps.Model, nodes.ProbeEvalOptions{}),
		)).
		AddNodeSpec(graph.PatchNode(
			nodes.NodeUpdateMemory,
			[]string{graph.WriteWorkingMemory, graph.WriteCurrentRoundCompletion},
			nodes.NewUpdateMemoryPatchNode(nodes.UpdateMemoryOptions{}),
		)).
		AddNode(nodes.NodeUpdateDifficulty, nodes.NewUpdateDifficultyNode(nodes.UpdateDifficultyOptions{})).
		AddNode(nodeReflectionCheck, nodes.NewReflectionCheckNode(deps.Model, nodes.ReflectionCheckOptions{})).
		AddNodeSpec(graph.PatchNode(
			nodes.NodeReport,
			[]string{graph.WritePendingDecision, graph.WriteReport, graph.WriteStatus, graph.WriteWorkingMemory},
			nodes.NewReportPatchNodeWithHook(nil),
		)).
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
