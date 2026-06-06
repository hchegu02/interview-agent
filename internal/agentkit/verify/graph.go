package verify

import (
	"context"
	"fmt"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/embedding"
	"interview-agent/internal/graph"
	"interview-agent/internal/graphs"
	"interview-agent/internal/llm"
	"interview-agent/internal/nodes"
	"interview-agent/internal/retriever"
)

type GraphStructureVerifier struct{}

func (GraphStructureVerifier) VerifyInterviewGraph() []Failure {
	r, err := graphs.BuildInterviewGraph(graphs.Deps{
		Model:     verifyChatModel{},
		Embedder:  verifyEmbedder{},
		Retriever: verifyRetriever{},
	})
	if err != nil {
		return []Failure{{Code: "graph_compile_failed", Message: err.Error(), Target: "interview_graph"}}
	}

	failures := []Failure{}
	specs := map[string]graph.NodeSpec{}
	for _, spec := range r.NodeSpecs() {
		specs[spec.Name] = spec
	}

	expectedPatchWrites := map[string][]string{
		nodes.NodeUpdateMemory:     {graph.WriteWorkingMemory, graph.WriteCurrentRoundCompletion},
		nodes.NodeUpdateDifficulty: {graph.WriteWorkingMemory},
		"reflection_check":         {graph.WritePendingDecision, graph.WriteWorkingMemory},
	}
	for node, writes := range expectedPatchWrites {
		spec, ok := specs[node]
		if !ok {
			failures = append(failures, Failure{Code: "graph_node_missing", Message: "required graph node is missing", Target: node})
			continue
		}
		if !spec.IsPatchNode() {
			failures = append(failures, Failure{Code: "graph_node_not_patch", Message: "required graph node is not registered as PatchNode", Target: node})
		}
		if missing := missingStrings(writes, spec.Writes); len(missing) > 0 {
			failures = append(failures, Failure{
				Code:    "graph_write_set_missing",
				Message: fmt.Sprintf("missing declared writes: %v", missing),
				Target:  node,
			})
		}
	}

	if !containsString(r.Successors(nodes.NodeUpdateMemory), nodes.NodeUpdateDifficulty) {
		failures = append(failures, Failure{Code: "graph_order_invalid", Message: "update_memory must lead to update_difficulty", Target: nodes.NodeUpdateMemory})
	}
	if !containsString(r.Successors(nodes.NodeUpdateDifficulty), "reflection_check") {
		failures = append(failures, Failure{Code: "graph_order_invalid", Message: "update_difficulty must lead to reflection_check", Target: nodes.NodeUpdateDifficulty})
	}
	if !r.HasRouter("reflection_check") {
		failures = append(failures, Failure{Code: "graph_router_missing", Message: "reflection_check must route to pick_next or report", Target: "reflection_check"})
	}

	failures = append(failures, verifyCumulativeNodeIdempotency()...)
	return failures
}

func verifyCumulativeNodeIdempotency() []Failure {
	failures := []Failure{}
	if failure := verifyUpdateMemoryIdempotency(); failure != nil {
		failures = append(failures, *failure)
	}
	if failure := verifyUpdateDifficultyIdempotency(); failure != nil {
		failures = append(failures, *failure)
	}
	if failure := verifyReflectionCheckIdempotency(); failure != nil {
		failures = append(failures, *failure)
	}
	return failures
}

func verifyUpdateMemoryIdempotency() *Failure {
	sess := &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{{
			RoundID:    "r1",
			Question:   domain.Question{ID: "q1", SkillCategory: "go"},
			Evaluation: &domain.Evaluation{QuestionID: "q1", Score: 80},
		}},
	}
	node := nodes.NewUpdateMemoryPatchNode(nodes.UpdateMemoryOptions{})
	if err := applyPatchNodeTwice(sess, node); err != nil {
		return &Failure{Code: "graph_idempotency_failed", Message: err.Error(), Target: nodes.NodeUpdateMemory}
	}
	if sess.WorkingMemory.ScoredRounds != 1 || sess.WorkingMemory.SkillCoverage["go"] != 0.8 {
		return &Failure{Code: "graph_idempotency_failed", Message: "update_memory duplicated cumulative scoring", Target: nodes.NodeUpdateMemory}
	}
	return nil
}

func verifyUpdateDifficultyIdempotency() *Failure {
	sess := &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds: []domain.AnswerRound{{
			RoundID:    "r1",
			Question:   domain.Question{ID: "q1", SkillCategory: "go"},
			Evaluation: &domain.Evaluation{QuestionID: "q1", Score: 85},
		}},
	}
	sess.Rounds[0].CompletedAt = time.Unix(1, 0).UTC()
	node := nodes.NewUpdateDifficultyPatchNode(nodes.UpdateDifficultyOptions{})
	if err := applyPatchNodeTwice(sess, node); err != nil {
		return &Failure{Code: "graph_idempotency_failed", Message: err.Error(), Target: nodes.NodeUpdateDifficulty}
	}
	if sess.WorkingMemory.Difficulty == nil || sess.WorkingMemory.Difficulty.CorrectStreak != 1 {
		return &Failure{Code: "graph_idempotency_failed", Message: "update_difficulty duplicated streak update", Target: nodes.NodeUpdateDifficulty}
	}
	return nil
}

func verifyReflectionCheckIdempotency() *Failure {
	sess := &domain.Session{
		WorkingMemory: domain.NewWorkingMemory(),
		Rounds:        []domain.AnswerRound{{RoundID: "r1"}},
	}
	sess.WorkingMemory.RoundsAsked = 3
	sess.WorkingMemory.MaxRounds = 8
	sess.WorkingMemory.WeakSkills = []string{"redis"}
	node := nodes.NewReflectionCheckPatchNode(nil, nodes.ReflectionCheckOptions{})
	if err := applyPatchNodeTwice(sess, node); err != nil {
		return &Failure{Code: "graph_idempotency_failed", Message: err.Error(), Target: "reflection_check"}
	}
	if sess.WorkingMemory.ReflectionsUsed != 1 {
		return &Failure{Code: "graph_idempotency_failed", Message: "reflection_check duplicated reflection usage", Target: "reflection_check"}
	}
	return nil
}

func applyPatchNodeTwice(sess *domain.Session, node graph.PatchNodeFunc) error {
	for i := 0; i < 2; i++ {
		patch, err := node(context.Background(), sess)
		if err != nil {
			return err
		}
		if err := domain.ApplyStatePatch(sess, patch); err != nil {
			return err
		}
	}
	return nil
}

func missingStrings(want, got []string) []string {
	missing := []string{}
	for _, value := range want {
		if !containsString(got, value) {
			missing = append(missing, value)
		}
	}
	return missing
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type verifyChatModel struct{}

func (verifyChatModel) Generate(ctx context.Context, messages []llm.Message, opts llm.Options) (*llm.Response, error) {
	return nil, fmt.Errorf("verify model should not be called")
}

func (verifyChatModel) Stream(ctx context.Context, messages []llm.Message, opts llm.Options) (<-chan llm.Chunk, error) {
	return nil, fmt.Errorf("verify model should not be called")
}

func (verifyChatModel) Name() string { return "verify-stub" }

type verifyEmbedder struct{}

func (verifyEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return nil, fmt.Errorf("verify embedder should not be called")
}

func (verifyEmbedder) Dimension() int { return 3 }

func (verifyEmbedder) Name() string { return "verify-stub" }

type verifyRetriever struct{}

func (verifyRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	return nil, fmt.Errorf("verify retriever should not be called")
}

var (
	_ llm.ChatModel       = verifyChatModel{}
	_ embedding.Embedder  = verifyEmbedder{}
	_ retriever.Retriever = verifyRetriever{}
)
