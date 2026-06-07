package questionbank

import (
	"context"
	"sync"
)

type GenerationJobStore interface {
	Create(ctx context.Context, job GenerationJob) (GenerationJob, error)
	Update(ctx context.Context, job GenerationJob) (GenerationJob, error)
	Get(ctx context.Context, id string) (GenerationJob, error)
}

type MemoryGenerationJobStore struct {
	mu   sync.Mutex
	jobs map[string]GenerationJob
}

func NewMemoryGenerationJobStore() *MemoryGenerationJobStore {
	return &MemoryGenerationJobStore{jobs: map[string]GenerationJob{}}
}

func (s *MemoryGenerationJobStore) Create(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	_ = ctx
	if s == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[string]GenerationJob{}
	}
	s.jobs[job.ID] = cloneGenerationJob(job)
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) Update(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	_ = ctx
	if s == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[string]GenerationJob{}
	}
	if _, ok := s.jobs[job.ID]; !ok {
		return GenerationJob{}, ErrImportNotFound
	}
	s.jobs[job.ID] = cloneGenerationJob(job)
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) Get(ctx context.Context, id string) (GenerationJob, error) {
	_ = ctx
	if s == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return GenerationJob{}, ErrImportNotFound
	}
	return cloneGenerationJob(job), nil
}
