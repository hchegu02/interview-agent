package questionbank

import (
	"context"
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
