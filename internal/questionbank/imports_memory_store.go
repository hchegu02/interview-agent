package questionbank

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryImportStore struct {
	mu     sync.Mutex
	jobs   map[string]ImportJob
	chunks map[string][]ImportChunk
	items  map[string][]ImportItem
}

func NewMemoryImportStore() *MemoryImportStore {
	return &MemoryImportStore{
		jobs:   map[string]ImportJob{},
		chunks: map[string][]ImportChunk{},
		items:  map[string][]ImportItem{},
	}
}

func (s *MemoryImportStore) CreateJob(_ context.Context, job ImportJob) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) UpdateJob(_ context.Context, job ImportJob) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return ImportJob{}, ErrImportNotFound
	}
	job.UpdatedAt = time.Now().UTC()
	s.jobs[job.ID] = job
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) GetJob(_ context.Context, id string) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ImportJob{}, ErrImportNotFound
	}
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) ListJobs(_ context.Context) ([]ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ImportJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, cloneImportJob(job))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryImportStore) AddChunks(_ context.Context, chunks []ImportChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chunk := range chunks {
		s.chunks[chunk.JobID] = append(s.chunks[chunk.JobID], cloneImportChunk(chunk))
	}
	return nil
}

func (s *MemoryImportStore) ListChunks(_ context.Context, jobID string) ([]ImportChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]ImportChunk(nil), s.chunks[jobID]...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (s *MemoryImportStore) AddItems(_ context.Context, items []ImportItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.items[item.JobID] = append(s.items[item.JobID], cloneImportItem(item))
	}
	return nil
}

func (s *MemoryImportStore) ListItems(_ context.Context, jobID string) ([]ImportItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]ImportItem(nil), s.items[jobID]...)
	for i := range out {
		out[i] = cloneImportItem(out[i])
	}
	return out, nil
}

func (s *MemoryImportStore) UpdateItems(_ context.Context, items []ImportItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		for jobID := range s.items {
			for i := range s.items[jobID] {
				if s.items[jobID][i].ID == item.ID {
					s.items[jobID][i] = cloneImportItem(item)
				}
			}
		}
	}
	return nil
}

func (s *MemoryImportStore) UpdateItemReviews(_ context.Context, jobID string, itemIDs []string, reviewStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := map[string]struct{}{}
	for _, id := range itemIDs {
		wanted[id] = struct{}{}
	}
	for i := range s.items[jobID] {
		if _, ok := wanted[s.items[jobID][i].ID]; !ok {
			continue
		}
		if s.items[jobID][i].Status != ImportItemStatusValid {
			continue
		}
		s.items[jobID][i].ReviewStatus = normalizedImportReviewStatus(reviewStatus)
		s.items[jobID][i].AgentReviewStatus = agentReviewStatusAfterHumanReview(s.items[jobID][i].AgentReviewStatus, reviewStatus)
		s.items[jobID][i].UpdatedAt = time.Now().UTC()
	}
	return nil
}

// 重置作业数据，删除所有与作业相关的切片和题目，但保留作业本身。这在重新处理同一文件时很有用，可以避免重复创建作业并保留作业的元数据和状态。
func (s *MemoryImportStore) ResetJobData(_ context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chunks, jobID)
	delete(s.items, jobID)
	return nil
}

// 检查作业是否存在，是否被其他人锁定，如果未锁定或已被当前用户锁定，则锁定作业并返回 true；如果被其他人锁定，则返回 false。
func (s *MemoryImportStore) TryAcquireJob(_ context.Context, jobID, ownerID string, leaseFor time.Duration) (ImportJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return ImportJob{}, false, ErrImportNotFound
	}
	now := time.Now().UTC()
	if job.OwnerID != "" && job.OwnerID != ownerID && job.LeaseUntil.After(now) {
		return cloneImportJob(job), false, nil
	}
	if leaseFor <= 0 {
		leaseFor = 2 * time.Minute
	}
	job.OwnerID = ownerID
	job.LeaseUntil = now.Add(leaseFor)
	job.UpdatedAt = now
	s.jobs[job.ID] = job
	return cloneImportJob(job), true, nil
}
