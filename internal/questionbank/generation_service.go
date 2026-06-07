package questionbank

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"interview-agent/internal/llm"
)

type GenerationService struct {
	imports     ImportStore
	writer      Writer
	model       llm.ChatModel
	jobs        GenerationJobStore
	workers     chan struct{}
	asyncMu     sync.Mutex
	asyncClosed bool
	asyncWG     sync.WaitGroup
}

type GenerationServiceDeps struct {
	Imports ImportStore
	Writer  Writer
	Model   llm.ChatModel
	Jobs    GenerationJobStore
}

func NewGenerationService(deps GenerationServiceDeps) *GenerationService {
	jobs := deps.Jobs
	if jobs == nil {
		jobs = NewMemoryGenerationJobStore()
	}
	return &GenerationService{
		imports: deps.Imports,
		writer:  deps.Writer,
		model:   deps.Model,
		jobs:    jobs,
		workers: make(chan struct{}, 2),
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
	if _, err := s.createJob(ctx, job); err != nil {
		return GenerationJob{}, err
	}
	return s.runGeneration(ctx, job)
}

func (s *GenerationService) EnqueueGenerate(ctx context.Context, req GenerationRequest) (GenerationJob, error) {
	if s == nil || s.imports == nil {
		return GenerationJob{}, errors.New("question generation store not configured")
	}
	if s.isShutdown() {
		return GenerationJob{}, ErrImportServiceShutdown
	}
	if err := validateGenerationRequest(req); err != nil {
		return GenerationJob{}, err
	}
	now := time.Now().UTC()
	job := GenerationJob{
		ID:        generationJobID(req, now),
		Status:    GenerationStatusQueued,
		Request:   req,
		CreatedAt: now,
		UpdatedAt: now,
	}
	job, err := s.createJob(ctx, job)
	if err != nil {
		return GenerationJob{}, err
	}
	s.runAsync(func() {
		_, _ = s.runGeneration(context.Background(), job)
	})
	return job, nil
}

func (s *GenerationService) runGeneration(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	req := job.Request
	job.Status = GenerationStatusRetrieving
	job.Error = ""
	job.UpdatedAt = time.Now().UTC()
	job = s.saveJob(ctx, job)
	chunks, err := retrieveGenerationChunks(ctx, s.imports, req, generationChunkLimit(req))
	if err != nil {
		return s.failGenerationJob(ctx, job, err), err
	}
	if len(chunks) == 0 {
		err := fmt.Errorf("no source chunks matched generation request")
		return s.failGenerationJob(ctx, job, err), err
	}

	job.Status = GenerationStatusDrafting
	job.UpdatedAt = time.Now().UTC()
	job = s.saveJob(ctx, job)
	concepts, warnings, err := s.extractConceptCards(ctx, req, chunks)
	if err != nil {
		return s.failGenerationJob(ctx, job, err), err
	}
	if len(concepts) == 0 {
		err := fmt.Errorf("no grounded concept cards generated")
		return s.failGenerationJob(ctx, job, err), err
	}

	drafts, err := s.generateQuestionCandidates(ctx, req, concepts, chunks)
	if err != nil {
		return s.failGenerationJob(ctx, job, err), err
	}
	job.Status = GenerationStatusGating
	job.UpdatedAt = time.Now().UTC()
	job = s.saveJob(ctx, job)
	existingContentKeys, err := activeQuestionContentKeys(ctx, s.writer)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("existing question dedupe skipped: %v", err))
	}
	passed, rejected := gateQuestionCandidates(req, concepts, chunks, drafts, existingContentKeys)
	if len(passed) == 0 {
		err := fmt.Errorf("no generated question candidates passed quality gates")
		job.Concepts = concepts
		job.RejectedCandidates = rejected
		job.Warnings = warnings
		return s.failGenerationJob(ctx, job, err), err
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
	job = s.saveJob(ctx, job)
	return job, nil
}

func (s *GenerationService) Get(ctx context.Context, id string) (GenerationJob, error) {
	if s == nil || s.jobs == nil {
		return GenerationJob{}, errors.New("question generation store not configured")
	}
	return s.jobs.Get(ctx, id)
}

func (s *GenerationService) runAsync(fn func()) bool {
	if s == nil {
		return false
	}
	s.asyncMu.Lock()
	if s.asyncClosed {
		s.asyncMu.Unlock()
		return false
	}
	s.asyncWG.Add(1)
	s.asyncMu.Unlock()

	go func() {
		defer s.asyncWG.Done()
		s.workers <- struct{}{}
		defer func() { <-s.workers }()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("question bank generation background task panicked: %v", recovered)
			}
		}()
		fn()
	}()
	return true
}

func (s *GenerationService) isShutdown() bool {
	if s == nil {
		return true
	}
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()
	return s.asyncClosed
}

func (s *GenerationService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.asyncMu.Lock()
	s.asyncClosed = true
	s.asyncMu.Unlock()

	done := make(chan struct{})
	go func() {
		s.asyncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *GenerationService) Stage(ctx context.Context, id string) (GenerationJob, ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return GenerationJob{}, ImportJob{}, nil, errors.New("question generation store not configured")
	}
	job, err := s.Get(ctx, id)
	if err != nil {
		return GenerationJob{}, ImportJob{}, nil, err
	}
	if job.Status != GenerationStatusCreated && job.Status != GenerationStatusStaged {
		return job, ImportJob{}, nil, fmt.Errorf("generation job %s is %s, not created", job.ID, job.Status)
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
	job = s.saveJob(ctx, job)
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

func (s *GenerationService) failGenerationJob(ctx context.Context, job GenerationJob, err error) GenerationJob {
	job.Status = GenerationStatusFailed
	job.Error = err.Error()
	job.UpdatedAt = time.Now().UTC()
	job = s.saveJob(ctx, job)
	return job
}

func (s *GenerationService) createJob(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	if s == nil || s.jobs == nil {
		return GenerationJob{}, errors.New("question generation store not configured")
	}
	return s.jobs.Create(ctx, cloneGenerationJob(job))
}

func (s *GenerationService) saveJob(ctx context.Context, job GenerationJob) GenerationJob {
	if s == nil || s.jobs == nil {
		return job
	}
	updated, err := s.jobs.Update(ctx, cloneGenerationJob(job))
	if err != nil {
		log.Printf("question bank generation job update failed: %v", err)
		return job
	}
	return updated
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
