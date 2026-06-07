package questionbank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"interview-agent/internal/llm"
)

type GenerationService struct {
	imports ImportStore
	writer  Writer
	model   llm.ChatModel
	mu      sync.Mutex
	jobs    map[string]GenerationJob
}

type GenerationServiceDeps struct {
	Imports ImportStore
	Writer  Writer
	Model   llm.ChatModel
}

func NewGenerationService(deps GenerationServiceDeps) *GenerationService {
	return &GenerationService{
		imports: deps.Imports,
		writer:  deps.Writer,
		model:   deps.Model,
		jobs:    map[string]GenerationJob{},
	}
}

func (s *GenerationService) Generate(ctx context.Context, req GenerationRequest) (GenerationJob, error) {
	if s == nil || s.imports == nil {
		return GenerationJob{}, errors.New("question generation store not configured")
	}
	if err := validateGenerationRequest(req); err != nil {
		return GenerationJob{}, err
	}
	now := time.Now().UTC()
	job := GenerationJob{
		ID:        generationJobID(req, now),
		Status:    GenerationStatusRetrieving,
		Request:   req,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.saveJob(job)

	chunks, err := retrieveGenerationChunks(ctx, s.imports, req, generationChunkLimit(req))
	if err != nil {
		return s.failGenerationJob(job, err), err
	}
	if len(chunks) == 0 {
		err := fmt.Errorf("no source chunks matched generation request")
		return s.failGenerationJob(job, err), err
	}

	job.Status = GenerationStatusDrafting
	job.UpdatedAt = time.Now().UTC()
	s.saveJob(job)
	concepts, warnings, err := s.extractConceptCards(ctx, req, chunks)
	if err != nil {
		return s.failGenerationJob(job, err), err
	}
	if len(concepts) == 0 {
		err := fmt.Errorf("no grounded concept cards generated")
		return s.failGenerationJob(job, err), err
	}

	drafts, err := s.generateQuestionCandidates(ctx, req, concepts, chunks)
	if err != nil {
		return s.failGenerationJob(job, err), err
	}
	job.Status = GenerationStatusGating
	job.UpdatedAt = time.Now().UTC()
	s.saveJob(job)
	passed, rejected := gateQuestionCandidates(req, concepts, chunks, drafts)
	if len(passed) == 0 {
		err := fmt.Errorf("no generated question candidates passed quality gates")
		job.Concepts = concepts
		job.RejectedCandidates = rejected
		job.Warnings = warnings
		return s.failGenerationJob(job, err), err
	}
	for i := range passed {
		passed[i].ID = questionCandidateID(job.ID, i, passed[i])
	}
	for i := range rejected {
		if rejected[i].ID == "" {
			rejected[i].ID = questionCandidateID(job.ID, i, rejected[i])
		}
	}
	job.Status = GenerationStatusCreated
	job.Concepts = concepts
	job.Candidates = passed
	job.RejectedCandidates = rejected
	job.Warnings = warnings
	job.UpdatedAt = time.Now().UTC()
	s.saveJob(job)
	return job, nil
}

func (s *GenerationService) Get(ctx context.Context, id string) (GenerationJob, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return GenerationJob{}, ErrImportNotFound
	}
	return cloneGenerationJob(job), nil
}

func (s *GenerationService) Stage(ctx context.Context, id string) (GenerationJob, ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return GenerationJob{}, ImportJob{}, nil, errors.New("question generation store not configured")
	}
	job, err := s.Get(ctx, id)
	if err != nil {
		return GenerationJob{}, ImportJob{}, nil, err
	}
	if len(job.Candidates) == 0 {
		return job, ImportJob{}, nil, errors.New("generation job has no accepted candidates")
	}
	if job.StagedImportJobID != "" {
		importJob, err := s.imports.GetJob(ctx, job.StagedImportJobID)
		if err != nil {
			return job, ImportJob{}, nil, err
		}
		items, err := s.imports.ListItems(ctx, importJob.ID)
		return job, importJob, items, err
	}
	importJob, err := s.imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "generated-"+job.ID+".json"))
	if err != nil {
		return job, ImportJob{}, nil, err
	}
	importJob.Metadata = map[string]string{
		"generation_job_id": job.ID,
		"source_job_id":     job.Request.SourceJobID,
		"metadata_version":  GeneratedQuestionMetadataVersion,
	}
	chunkID := "generation:" + job.ID
	if err := s.imports.AddChunks(ctx, []ImportChunk{{
		ID:        chunkID,
		JobID:     importJob.ID,
		Index:     0,
		Content:   generationImportChunkContent(job),
		Metadata:  cloneStringMap(importJob.Metadata),
		CreatedAt: time.Now().UTC(),
	}}); err != nil {
		return job, importJob, nil, err
	}
	importJob.TotalChunks = 1
	importJob, _ = s.imports.UpdateJob(ctx, importJob)

	items, provenances := generationCandidatesToImportItems(job)
	stager := &ImportService{imports: s.imports, writer: s.writer}
	importJob, err = stager.stageItemsWithOriginalsAndProvenance(ctx, importJob, chunkID, items, nil, provenances)
	if err != nil {
		return job, importJob, nil, err
	}
	importJob.Status = ImportStatusReady
	importJob, err = s.imports.UpdateJob(ctx, importJob)
	if err != nil {
		return job, importJob, nil, err
	}
	staged, err := s.imports.ListItems(ctx, importJob.ID)
	if err != nil {
		return job, importJob, nil, err
	}
	job.Status = GenerationStatusStaged
	job.StagedImportJobID = importJob.ID
	job.UpdatedAt = time.Now().UTC()
	s.saveJob(job)
	return job, importJob, staged, nil
}

func generationChunkLimit(req GenerationRequest) int {
	limit := req.Count * 3
	if limit < 8 {
		return 8
	}
	if limit > 24 {
		return 24
	}
	return limit
}

func generationJobID(req GenerationRequest, now time.Time) string {
	return importGeneratedID("gen", fmt.Sprintf("%s:%s:%s:%d:%d:%d", req.SourceJobID, req.Topic, req.QuestionType, req.Count, req.Difficulty, now.UnixNano()))
}

func questionCandidateID(jobID string, index int, candidate QuestionCandidate) string {
	return importGeneratedID("gq", fmt.Sprintf("%s:%03d:%s:%s", jobID, index, candidate.ConceptID, candidate.Content))
}

func (s *GenerationService) failGenerationJob(job GenerationJob, err error) GenerationJob {
	job.Status = GenerationStatusFailed
	job.Error = err.Error()
	job.UpdatedAt = time.Now().UTC()
	s.saveJob(job)
	return job
}

func (s *GenerationService) saveJob(job GenerationJob) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[string]GenerationJob{}
	}
	s.jobs[job.ID] = cloneGenerationJob(job)
}

func cloneGenerationJob(job GenerationJob) GenerationJob {
	job.Concepts = append([]ConceptCard(nil), job.Concepts...)
	job.Candidates = append([]QuestionCandidate(nil), job.Candidates...)
	job.RejectedCandidates = append([]QuestionCandidate(nil), job.RejectedCandidates...)
	job.Warnings = append([]string(nil), job.Warnings...)
	return job
}

func generationCandidatesToImportItems(job GenerationJob) ([]Item, []map[string]string) {
	items := make([]Item, 0, len(job.Candidates))
	provenances := make([]map[string]string, 0, len(job.Candidates))
	for _, candidate := range job.Candidates {
		item := Item{
			ID:             candidate.ID,
			Content:        strings.TrimSpace(candidate.Content),
			Tags:           compactStrings(append(append([]string(nil), job.Request.Tags...), candidate.Tags...)),
			SkillCategory:  firstNonEmpty(candidate.SkillCategory, job.Request.SkillCategory, "general"),
			Difficulty:     candidate.Difficulty,
			ExpectedPoints: append([]string(nil), candidate.ExpectedPoints...),
			Source:         "generated",
			Scenario:       candidate.TargetDimension,
			Rubric:         cloneStringMap(candidate.Rubric),
			SampleAnswer:   candidate.SampleAnswer,
			FollowUpHints:  append([]string(nil), candidate.FollowUpHints...),
			Status:         "active",
		}
		items = append(items, item)
		provenances = append(provenances, generatedQuestionProvenance(job, candidate))
	}
	return items, provenances
}

func generatedQuestionProvenance(job GenerationJob, candidate QuestionCandidate) map[string]string {
	out := map[string]string{
		"metadata_version":  GeneratedQuestionMetadataVersion,
		"generation_job_id": job.ID,
		"source_job_id":     job.Request.SourceJobID,
		"candidate_id":      candidate.ID,
		"concept_id":        candidate.ConceptID,
		"question_type":     candidate.QuestionType,
		"answer":            candidate.Answer,
		"explanation":       candidate.Explanation,
	}
	for i, ref := range candidate.SourceRefs {
		key := fmt.Sprintf("source_ref_%02d", i)
		out[key] = strings.TrimSpace(ref.ChunkID) + ":" + strings.TrimSpace(ref.Quote)
	}
	return out
}

func generationImportChunkContent(job GenerationJob) string {
	var b strings.Builder
	b.WriteString("Generated question candidates for topic: ")
	b.WriteString(job.Request.Topic)
	for _, concept := range job.Concepts {
		b.WriteString("\n- ")
		b.WriteString(concept.Title)
		for _, ref := range concept.EvidenceRefs {
			b.WriteString("\n  ")
			b.WriteString(ref.ChunkID)
			b.WriteString(": ")
			b.WriteString(ref.Quote)
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
