package questionbank

import (
	"context"
	"strings"
	"testing"
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
