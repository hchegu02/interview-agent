package questionbank

import (
	"errors"
	"fmt"
	"strings"
)

const (
	GenerationStatusCreated    = "created"
	GenerationStatusRetrieving = "retrieving"
	GenerationStatusDrafting   = "drafting"
	GenerationStatusGating     = "gating"
	GenerationStatusStaged     = "staged"
	GenerationStatusFailed     = "failed"

	GeneratedQuestionMetadataVersion = "generated_question_v1"
)

type GenerationRequest struct {
	SourceJobID     string   `json:"source_job_id"`
	Topic           string   `json:"topic"`
	QuestionType    string   `json:"question_type"`
	Count           int      `json:"count"`
	Difficulty      int      `json:"difficulty"`
	TargetDimension string   `json:"target_dimension,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	SkillCategory   string   `json:"skill_category,omitempty"`
}

type GenerationJob struct {
	ID         string              `json:"id"`
	Status     string              `json:"status"`
	Request    GenerationRequest   `json:"request"`
	Concepts   []ConceptCard       `json:"concepts,omitempty"`
	Candidates []QuestionCandidate `json:"candidates,omitempty"`
	Error      string              `json:"error,omitempty"`
}

type SourceRef struct {
	ChunkID string `json:"chunk_id"`
	Quote   string `json:"quote"`
}

type ConceptCard struct {
	ID             string      `json:"concept_id"`
	Title          string      `json:"title"`
	Skill          string      `json:"skill,omitempty"`
	SubSkill       string      `json:"sub_skill,omitempty"`
	DifficultyHint int         `json:"difficulty_hint,omitempty"`
	Keywords       []string    `json:"keywords,omitempty"`
	QuestionAngles []string    `json:"question_angles,omitempty"`
	EvidenceRefs   []SourceRef `json:"evidence_refs"`
}

type QuestionCandidate struct {
	ID              string            `json:"candidate_id,omitempty"`
	ConceptID       string            `json:"concept_id"`
	Content         string            `json:"content"`
	QuestionType    string            `json:"question_type"`
	TargetDimension string            `json:"target_dimension,omitempty"`
	Options         []string          `json:"options,omitempty"`
	Answer          string            `json:"answer,omitempty"`
	Explanation     string            `json:"explanation,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	SkillCategory   string            `json:"skill_category,omitempty"`
	Difficulty      int               `json:"difficulty,omitempty"`
	ExpectedPoints  []string          `json:"expected_points,omitempty"`
	Rubric          map[string]string `json:"rubric,omitempty"`
	SampleAnswer    string            `json:"sample_answer,omitempty"`
	FollowUpHints   []string          `json:"follow_up_hints,omitempty"`
	SourceRefs      []SourceRef       `json:"source_refs"`
	QualityFlags    []string          `json:"quality_flags,omitempty"`
}

func validateGenerationRequest(req GenerationRequest) error {
	if strings.TrimSpace(req.SourceJobID) == "" {
		return errors.New("source_job_id is required")
	}
	if strings.TrimSpace(req.Topic) == "" {
		return errors.New("topic is required")
	}
	if !validGenerationQuestionType(req.QuestionType) {
		return fmt.Errorf("unsupported question_type %q", req.QuestionType)
	}
	if req.Count < 1 || req.Count > 20 {
		return errors.New("count must be between 1 and 20")
	}
	if req.Difficulty < 1 || req.Difficulty > 5 {
		return errors.New("difficulty must be between 1 and 5")
	}
	if req.TargetDimension != "" && !validGenerationTargetDimension(req.TargetDimension) {
		return fmt.Errorf("unsupported target_dimension %q", req.TargetDimension)
	}
	return nil
}

func validGenerationQuestionType(v string) bool {
	switch strings.TrimSpace(v) {
	case "interview", "single_choice", "short_answer":
		return true
	default:
		return false
	}
}

func validGenerationTargetDimension(v string) bool {
	switch strings.TrimSpace(v) {
	case "concept", "principle", "scenario", "tradeoff", "debugging", "project_experience", "system_design":
		return true
	default:
		return false
	}
}
