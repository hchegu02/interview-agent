package questionbank

import (
	"context"
	"strings"
	"testing"

	"interview-agent/internal/llm"
)

func TestValidateGenerationRequestRejectsMissingRequiredFields(t *testing.T) {
	req := GenerationRequest{Topic: "Go 并发", Count: 5, Difficulty: 3, QuestionType: "interview"}
	if err := validateGenerationRequest(req); err == nil {
		t.Fatal("expected missing source_job_id to fail")
	}
}

func TestValidateGenerationRequestRejectsInvalidEnumsAndBounds(t *testing.T) {
	tests := []struct {
		name string
		req  GenerationRequest
	}{
		{
			name: "unsupported question type",
			req: GenerationRequest{
				SourceJobID:  "imp-001",
				Topic:        "Go 并发",
				QuestionType: "essay",
				Count:        5,
				Difficulty:   3,
			},
		},
		{
			name: "count too large",
			req: GenerationRequest{
				SourceJobID:  "imp-001",
				Topic:        "Go 并发",
				QuestionType: "interview",
				Count:        21,
				Difficulty:   3,
			},
		},
		{
			name: "invalid difficulty",
			req: GenerationRequest{
				SourceJobID:  "imp-001",
				Topic:        "Go 并发",
				QuestionType: "interview",
				Count:        5,
				Difficulty:   0,
			},
		},
		{
			name: "unsupported target dimension",
			req: GenerationRequest{
				SourceJobID:     "imp-001",
				Topic:           "Go 并发",
				QuestionType:    "interview",
				Count:           5,
				Difficulty:      3,
				TargetDimension: "random",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateGenerationRequest(tt.req); err == nil {
				t.Fatal("expected validation to fail")
			}
		})
	}
}

func TestValidateGenerationRequestAcceptsConceptFirstMVPFields(t *testing.T) {
	req := GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "Go 并发",
		QuestionType:    "interview",
		Count:           5,
		Difficulty:      3,
		TargetDimension: "debugging",
		SkillCategory:   "go",
		Tags:            []string{"go", "concurrency"},
	}
	if err := validateGenerationRequest(req); err != nil {
		t.Fatalf("validateGenerationRequest: %v", err)
	}
}

func TestGenerationRetrievalScopesChunksToSourceJob(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	if err := imports.AddChunks(ctx, []ImportChunk{
		{
			ID:      "imp-001:chunk:001",
			JobID:   "imp-001",
			Index:   1,
			Content: "Go 并发排查 goroutine 泄漏，需要 pprof goroutine 和 context 取消。",
		},
		{
			ID:      "imp-002:chunk:001",
			JobID:   "imp-002",
			Index:   1,
			Content: "Go 并发排查 goroutine 泄漏，但这是另一个来源作业。",
		},
	}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	chunks, err := retrieveGenerationChunks(ctx, imports, GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "goroutine 泄漏",
		QuestionType:    "interview",
		Count:           3,
		Difficulty:      3,
		TargetDimension: "debugging",
	}, 10)
	if err != nil {
		t.Fatalf("retrieveGenerationChunks: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1: %+v", len(chunks), chunks)
	}
	if chunks[0].ID != "imp-001:chunk:001" || chunks[0].JobID != "imp-001" {
		t.Fatalf("retrieved wrong chunk: %+v", chunks[0])
	}
}

func TestGenerationRetrievalReturnsEmptyWhenNoTermsMatch(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	if err := imports.AddChunks(ctx, []ImportChunk{
		{
			ID:      "imp-001:chunk:001",
			JobID:   "imp-001",
			Index:   1,
			Content: "Redis 缓存击穿需要互斥锁或逻辑过期。",
		},
	}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	chunks, err := retrieveGenerationChunks(ctx, imports, GenerationRequest{
		SourceJobID:  "imp-001",
		Topic:        "Kubernetes 调度器",
		QuestionType: "interview",
		Count:        3,
		Difficulty:   3,
	}, 10)
	if err != nil {
		t.Fatalf("retrieveGenerationChunks: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("chunks = %+v, want empty", chunks)
	}
}

func TestValidateConceptCardsKeepsGroundedConcepts(t *testing.T) {
	chunks := []RetrievedChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Content: "goroutine 泄漏通常来自阻塞发送、未取消 context 或慢客户端堆积。",
	}}
	cards, rejected := validateConceptCards("gen-001", []ConceptCard{{
		Title:          "goroutine 泄漏",
		Skill:          "并发治理",
		DifficultyHint: 3,
		EvidenceRefs: []SourceRef{{
			ChunkID: "imp-001:chunk:001",
			Quote:   "未取消 context",
		}},
	}}, chunks)
	if len(rejected) != 0 {
		t.Fatalf("rejected = %+v, want none", rejected)
	}
	if len(cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(cards))
	}
	if cards[0].ID == "" || !strings.HasPrefix(cards[0].ID, "concept-") {
		t.Fatalf("backend concept id not generated: %+v", cards[0])
	}
}

func TestValidateConceptCardsRejectsForeignOrUngroundedRefs(t *testing.T) {
	chunks := []RetrievedChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Content: "Redis 缓存击穿可以用互斥锁或逻辑过期治理。",
	}}
	cards, rejected := validateConceptCards("gen-001", []ConceptCard{
		{
			Title:        "foreign",
			EvidenceRefs: []SourceRef{{ChunkID: "imp-002:chunk:001", Quote: "互斥锁"}},
		},
		{
			Title:        "ungrounded",
			EvidenceRefs: []SourceRef{{ChunkID: "imp-001:chunk:001", Quote: "布隆过滤器"}},
		},
	}, chunks)
	if len(cards) != 0 {
		t.Fatalf("cards = %+v, want empty", cards)
	}
	if len(rejected) != 2 {
		t.Fatalf("rejected = %+v, want two reasons", rejected)
	}
}

func TestValidateConceptCardsDeduplicatesByTitleAndEvidence(t *testing.T) {
	chunks := []RetrievedChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Content: "缓存击穿是热点 key 过期后大量请求打到数据库的问题。",
	}}
	input := []ConceptCard{
		{
			Title:        "缓存击穿",
			EvidenceRefs: []SourceRef{{ChunkID: "imp-001:chunk:001", Quote: "热点 key 过期"}},
		},
		{
			Title:        " 缓存击穿 ",
			EvidenceRefs: []SourceRef{{ChunkID: "imp-001:chunk:001", Quote: "热点 key 过期"}},
		},
	}
	cards, rejected := validateConceptCards("gen-001", input, chunks)
	if len(cards) != 1 {
		t.Fatalf("cards = %+v, want one deduped card", cards)
	}
	if len(rejected) != 1 {
		t.Fatalf("rejected = %+v, want duplicate reason", rejected)
	}
}

func TestExtractConceptCardsFallbackUsesRetrievedChunks(t *testing.T) {
	ctx := context.Background()
	service := &GenerationService{}
	chunks := []RetrievedChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Content: "Go 并发治理需要识别 goroutine 泄漏，并用 pprof 和 context 取消定位问题。",
	}}

	cards, rejected, err := service.extractConceptCards(ctx, GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "goroutine 泄漏",
		QuestionType:    "interview",
		Count:           3,
		Difficulty:      4,
		TargetDimension: "debugging",
		SkillCategory:   "go",
	}, chunks)
	if err != nil {
		t.Fatalf("extractConceptCards: %v", err)
	}
	if len(rejected) != 0 {
		t.Fatalf("rejected = %+v, want none", rejected)
	}
	if len(cards) != 1 {
		t.Fatalf("cards = %+v, want one", cards)
	}
	if cards[0].Title != "goroutine 泄漏" || cards[0].Skill != "go" || cards[0].DifficultyHint != 4 {
		t.Fatalf("fallback concept mismatch: %+v", cards[0])
	}
}

func TestGateQuestionCandidatesAcceptsGroundedInterviewCandidate(t *testing.T) {
	req := GenerationRequest{QuestionType: "interview", Difficulty: 3}
	concepts := []ConceptCard{{
		ID:    "concept-001",
		Title: "goroutine 泄漏",
		EvidenceRefs: []SourceRef{{
			ChunkID: "chunk-1",
			Quote:   "context 取消",
		}},
	}}
	chunks := []RetrievedChunk{{ID: "chunk-1", Content: "goroutine 泄漏排查要看 context 取消和 pprof。"}}
	candidate := completeGenerationCandidate("concept-001", "如何排查 goroutine 泄漏？")

	passed, rejected := gateQuestionCandidates(req, concepts, chunks, []QuestionCandidate{candidate})
	if len(rejected) != 0 {
		t.Fatalf("rejected = %+v, want none", rejected)
	}
	if len(passed) != 1 {
		t.Fatalf("passed = %+v, want one", passed)
	}
}

func TestGateQuestionCandidatesRejectsMissingRefsUnknownConceptAndUngroundedQuote(t *testing.T) {
	req := GenerationRequest{QuestionType: "interview", Difficulty: 3}
	concepts := []ConceptCard{{ID: "concept-001", Title: "缓存击穿"}}
	chunks := []RetrievedChunk{{ID: "chunk-1", Content: "缓存击穿可以用互斥锁治理。"}}
	candidates := []QuestionCandidate{
		completeGenerationCandidate("concept-001", "无来源题"),
		completeGenerationCandidate("missing-concept", "未知能力点题"),
		completeGenerationCandidate("concept-001", "引用不落地题"),
	}
	candidates[0].SourceRefs = nil
	candidates[2].SourceRefs = []SourceRef{{ChunkID: "chunk-1", Quote: "逻辑过期"}}

	passed, rejected := gateQuestionCandidates(req, concepts, chunks, candidates)
	if len(passed) != 0 {
		t.Fatalf("passed = %+v, want empty", passed)
	}
	if len(rejected) != 3 {
		t.Fatalf("rejected = %+v, want three", rejected)
	}
	for _, item := range rejected {
		if len(item.QualityFlags) == 0 {
			t.Fatalf("rejected item missing quality flags: %+v", item)
		}
	}
}

func TestGateQuestionCandidatesRejectsDuplicatesAndLowValueSummary(t *testing.T) {
	req := GenerationRequest{QuestionType: "interview", Difficulty: 3}
	concepts := []ConceptCard{{ID: "concept-001", Title: "Agent 评估"}}
	chunks := []RetrievedChunk{{ID: "chunk-1", Content: "Agent 评估要看结果、过程和工程指标，也要看 context 取消等工程稳定性。"}}
	first := completeGenerationCandidate("concept-001", "Agent 效果如何评估？")
	duplicate := completeGenerationCandidate("concept-001", " Agent 效果如何评估？ ")
	lowValue := completeGenerationCandidate("concept-001", "请总结本文")

	passed, rejected := gateQuestionCandidates(req, concepts, chunks, []QuestionCandidate{first, duplicate, lowValue})
	if len(passed) != 1 {
		t.Fatalf("passed = %+v, want one", passed)
	}
	if len(rejected) != 2 {
		t.Fatalf("rejected = %+v, want two", rejected)
	}
}

func TestGateQuestionCandidatesRejectsInvalidSingleChoiceAndInterviewWithoutFollowups(t *testing.T) {
	concepts := []ConceptCard{{ID: "concept-001", Title: "缓存击穿"}}
	chunks := []RetrievedChunk{{ID: "chunk-1", Content: "缓存击穿可以用互斥锁和逻辑过期治理。"}}
	choice := completeGenerationCandidate("concept-001", "缓存击穿怎么治理？")
	choice.QuestionType = "single_choice"
	choice.Options = []string{"互斥锁", "逻辑过期"}
	choice.Answer = "A"
	interview := completeGenerationCandidate("concept-001", "缓存击穿怎么治理？")
	interview.FollowUpHints = nil

	_, rejectedChoice := gateQuestionCandidates(GenerationRequest{QuestionType: "single_choice", Difficulty: 3}, concepts, chunks, []QuestionCandidate{choice})
	if len(rejectedChoice) != 1 {
		t.Fatalf("rejectedChoice = %+v, want one", rejectedChoice)
	}
	_, rejectedInterview := gateQuestionCandidates(GenerationRequest{QuestionType: "interview", Difficulty: 3}, concepts, chunks, []QuestionCandidate{interview})
	if len(rejectedInterview) != 1 {
		t.Fatalf("rejectedInterview = %+v, want one", rejectedInterview)
	}
}

func TestParseQuestionCandidatesRequiresCandidatesEnvelope(t *testing.T) {
	if _, err := parseQuestionCandidatesJSON([]byte(`{"items":[]}`)); err == nil {
		t.Fatal("expected missing candidates envelope to fail")
	}
	got, err := parseQuestionCandidatesJSON([]byte(`{"candidates":[{"concept_id":"concept-001","content":"如何排查 goroutine 泄漏？","question_type":"interview","answer":"A","explanation":"E","tags":["go"],"skill_category":"go","difficulty":3,"expected_points":["p1"],"rubric":{"good":"ok"},"sample_answer":"S","follow_up_hints":["F"],"source_refs":[{"chunk_id":"chunk-1","quote":"context 取消"}]}]}`))
	if err != nil {
		t.Fatalf("parseQuestionCandidatesJSON: %v", err)
	}
	if len(got) != 1 || got[0].ConceptID != "concept-001" {
		t.Fatalf("candidates = %+v", got)
	}
}

func TestGenerationServiceGenerateProducesGroundedCandidates(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	if err := imports.AddChunks(ctx, []ImportChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Index:   1,
		Content: "Go 并发治理需要识别 goroutine 泄漏，并用 pprof 和 context 取消定位问题。",
	}}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
	service := NewGenerationService(GenerationServiceDeps{
		Imports: imports,
		Model: &scriptedGenerationModel{
			responses: []func([]llm.Message) string{
				func([]llm.Message) string {
					return `{"concepts":[{"title":"goroutine 泄漏","skill":"go","difficulty_hint":3,"keywords":["goroutine"],"question_angles":["debugging"],"evidence_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
				},
				func(messages []llm.Message) string {
					conceptID := conceptIDFromPrompt(messages)
					return `{"candidates":[{"concept_id":"` + conceptID + `","content":"如何排查 goroutine 泄漏？","question_type":"interview","target_dimension":"debugging","answer":"看 pprof 和 context。","explanation":"题目基于原文证据。","tags":["go","concurrency"],"skill_category":"go","difficulty":3,"expected_points":["说明 goroutine 泄漏现象","使用 pprof 定位","检查 context 取消"],"rubric":{"good":"能结合 pprof 和 context 说明排查路径"},"sample_answer":"先通过 pprof goroutine 查看阻塞点，再检查 context 是否取消。","follow_up_hints":["如果是慢客户端导致堆积怎么办？"],"source_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
				},
			},
		},
	})

	job, err := service.Generate(ctx, GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "goroutine 泄漏",
		QuestionType:    "interview",
		Count:           1,
		Difficulty:      3,
		TargetDimension: "debugging",
		SkillCategory:   "go",
		Tags:            []string{"go"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if job.Status != GenerationStatusCreated || len(job.Candidates) != 1 || len(job.RejectedCandidates) != 0 {
		t.Fatalf("job = %+v", job)
	}
	if !strings.HasPrefix(job.Candidates[0].ID, "gq-") {
		t.Fatalf("candidate id not generated: %+v", job.Candidates[0])
	}
}

func TestGenerationServiceStageRequiresHumanReviewBeforeCommit(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	store := NewMemoryStore(nil)
	if err := imports.AddChunks(ctx, []ImportChunk{{
		ID:      "imp-001:chunk:001",
		JobID:   "imp-001",
		Index:   1,
		Content: "Go 并发治理需要识别 goroutine 泄漏，并用 pprof 和 context 取消定位问题。",
	}}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}
	service := NewGenerationService(GenerationServiceDeps{
		Imports: imports,
		Writer:  store,
		Model: &scriptedGenerationModel{
			responses: []func([]llm.Message) string{
				func([]llm.Message) string {
					return `{"concepts":[{"title":"goroutine 泄漏","skill":"go","difficulty_hint":3,"keywords":["goroutine"],"question_angles":["debugging"],"evidence_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
				},
				func(messages []llm.Message) string {
					conceptID := conceptIDFromPrompt(messages)
					return `{"candidates":[{"concept_id":"` + conceptID + `","content":"如何排查 goroutine 泄漏？","question_type":"interview","target_dimension":"debugging","answer":"看 pprof 和 context。","explanation":"题目基于原文证据。","tags":["go"],"skill_category":"go","difficulty":3,"expected_points":["说明 goroutine 泄漏现象"],"rubric":{"good":"能说明 pprof 和 context"},"sample_answer":"通过 pprof 定位阻塞点，并检查 context 取消。","follow_up_hints":["如何避免再次泄漏？"],"source_refs":[{"chunk_id":"imp-001:chunk:001","quote":"context 取消"}]}]}`
				},
			},
		},
	})
	job, err := service.Generate(ctx, GenerationRequest{
		SourceJobID:     "imp-001",
		Topic:           "goroutine 泄漏",
		QuestionType:    "interview",
		Count:           1,
		Difficulty:      3,
		TargetDimension: "debugging",
		SkillCategory:   "go",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	stagedJob, importJob, items, err := service.Stage(ctx, job.ID)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if stagedJob.Status != GenerationStatusStaged || stagedJob.StagedImportJobID != importJob.ID {
		t.Fatalf("staged job = %+v importJob=%+v", stagedJob, importJob)
	}
	if importJob.Status != ImportStatusReady || len(items) != 1 {
		t.Fatalf("import job/items = %+v %+v", importJob, items)
	}
	if items[0].AgentReviewStatus != ImportAgentReviewNeedsHumanReview {
		t.Fatalf("agent review status = %q", items[0].AgentReviewStatus)
	}
	if items[0].SourceProvenance["metadata_version"] != GeneratedQuestionMetadataVersion {
		t.Fatalf("source provenance = %+v", items[0].SourceProvenance)
	}
	if items[0].SourceProvenance["answer"] == "" || items[0].SourceProvenance["explanation"] == "" {
		t.Fatalf("source provenance missing answer/explanation: %+v", items[0].SourceProvenance)
	}
	chunks, err := imports.ListChunks(ctx, importJob.ID)
	if err != nil {
		t.Fatalf("ListChunks: %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != items[0].ChunkID {
		t.Fatalf("generated import chunk mismatch: chunks=%+v item=%+v", chunks, items[0])
	}

	importService := NewImportService(ImportServiceDeps{Imports: imports, Writer: store})
	if _, _, err := importService.ReviewItems(ctx, importJob.ID, []string{items[0].ID}, ImportReviewStatusAccepted); err != nil {
		t.Fatalf("ReviewItems: %v", err)
	}
	if _, err := importService.Commit(ctx, importJob.ID); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := store.Get(ctx, items[0].QuestionID); err != nil {
		t.Fatalf("accepted generated item should be committed: %v", err)
	}
}

func completeGenerationCandidate(conceptID, content string) QuestionCandidate {
	return QuestionCandidate{
		ConceptID:      conceptID,
		Content:        content,
		QuestionType:   "interview",
		Answer:         "参考答案",
		Explanation:    "解析",
		Tags:           []string{"go"},
		SkillCategory:  "go",
		Difficulty:     3,
		ExpectedPoints: []string{"要点一", "要点二"},
		Rubric:         map[string]string{"good": "覆盖核心要点"},
		SampleAnswer:   "参考答案",
		FollowUpHints:  []string{"进一步追问边界条件"},
		SourceRefs:     []SourceRef{{ChunkID: "chunk-1", Quote: "context 取消"}},
	}
}

type scriptedGenerationModel struct {
	responses []func([]llm.Message) string
	calls     int
}

func (m *scriptedGenerationModel) Stream(context.Context, []llm.Message, llm.Options) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk)
	close(ch)
	return ch, nil
}

func (m *scriptedGenerationModel) Name() string { return "scripted-generation-model" }

func (m *scriptedGenerationModel) Generate(_ context.Context, messages []llm.Message, _ llm.Options) (*llm.Response, error) {
	if m.calls >= len(m.responses) {
		return &llm.Response{Content: `{"candidates":[]}`}, nil
	}
	content := m.responses[m.calls](messages)
	m.calls++
	return &llm.Response{Content: content}, nil
}

func conceptIDFromPrompt(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		content := messages[i].Content
		idx := strings.Index(content, `"concept_id":"concept-`)
		if idx < 0 {
			continue
		}
		start := idx + len(`"concept_id":"`)
		end := strings.Index(content[start:], `"`)
		if end > 0 {
			return content[start : start+end]
		}
	}
	return "concept-missing"
}
