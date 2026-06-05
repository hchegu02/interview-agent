package graphs

import (
	"context"
	"errors"
	"testing"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/retriever"
)

type stubChatModel struct {
	responses []string
	idx       int
}

func (s *stubChatModel) Name() string { return "stub" }

func (s *stubChatModel) Generate(ctx context.Context, messages []llm.Message, opts llm.Options) (*llm.Response, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("stub: no more responses")
	}
	resp := s.responses[s.idx]
	s.idx++
	return &llm.Response{Content: resp, Model: "stub"}, nil
}

func (s *stubChatModel) Stream(ctx context.Context, messages []llm.Message, opts llm.Options) (<-chan llm.Chunk, error) {
	return nil, errors.New("stub: stream not supported")
}

type stubEmbedder struct{}

func (s stubEmbedder) Name() string   { return "stub" }
func (s stubEmbedder) Dimension() int { return 1024 }
func (s stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0}
	}
	return out, nil
}

type stubRetriever struct {
	last retriever.Query
}

func (s *stubRetriever) Retrieve(ctx context.Context, q retriever.Query) ([]retriever.Result, error) {
	s.last = q
	return []retriever.Result{
		{
			ID:         "q1",
			Content:    "讲一下 Go GMP",
			Tags:       []string{"go"},
			Difficulty: 3,
			Category:   "go",
			Score:      0.9,
		},
	}, nil
}

func TestBuildInterviewGraph_InvokeRunsSetupAndSuspendsAtPickNext(t *testing.T) {
	model := &stubChatModel{responses: []string{
		`{"title":"Go 后端","level":"junior","key_skills":["go"],"must_have":["go"],"nice_to_have":[],"years_required":1}`,
		`{"years":2,"skills":["go"],"projects":[],"highlights":["做过 Go 服务"]}`,
		`{"next_question_id":"q1","reasoning":"先验证 Go 基础"}`,
	}}
	ret := &stubRetriever{}
	rec := graph.NewMemoryCheckpointRecorder(50)

	r, err := BuildInterviewGraph(Deps{
		Model:              model,
		Embedder:           stubEmbedder{},
		Retriever:          ret,
		CheckpointRecorder: rec,
	})
	if err != nil {
		t.Fatalf("build graph: %v", err)
	}

	sess := &domain.Session{
		ID:     "sess-graph",
		Status: domain.StatusRunning,
		JobProfile: &domain.JobProfile{
			JDRawText: "需要 Go 后端",
		},
		CandProfile: &domain.CandidateProfile{
			ResumeRawText: "两年 Go 开发经验",
		},
		WorkingMemory: domain.NewWorkingMemory(),
	}

	if err := r.Invoke(context.Background(), sess); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if sess.JobProfile.Title != "Go 后端" {
		t.Fatalf("job profile was not parsed: %+v", sess.JobProfile)
	}
	if sess.CandProfile.Years != 2 {
		t.Fatalf("candidate profile was not parsed: %+v", sess.CandProfile)
	}
	if sess.GapReport == nil {
		t.Fatal("gap report was not written")
	}
	if sess.ProfileAnalysis == nil {
		t.Fatal("profile analysis was not written")
	}
	if len(sess.CandidatePool) != 1 || sess.CandidatePool[0].ID != "q1" {
		t.Fatalf("candidate pool not loaded: %+v", sess.CandidatePool)
	}
	if len(sess.Rounds) != 1 || sess.Rounds[0].Question.ID != "q1" {
		t.Fatalf("pick_next did not create q1 round: %+v", sess.Rounds)
	}
	if sess.CurrentNode != "pick_next" {
		t.Fatalf("current node = %q, want pick_next", sess.CurrentNode)
	}
	if model.idx != 3 {
		t.Fatalf("LLM calls = %d, want 3", model.idx)
	}
	if ret.last.K == 0 {
		t.Fatalf("retriever was not called: %+v", ret.last)
	}
	if len(rec.Snapshot()) == 0 {
		t.Fatal("checkpoint recorder was not wired into interview graph")
	}
}
