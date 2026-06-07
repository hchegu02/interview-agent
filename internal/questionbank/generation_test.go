package questionbank

import "testing"

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
