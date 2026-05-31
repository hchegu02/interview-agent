package questionbank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/embedding"
)

func (s *ImportService) Commit(ctx context.Context, jobID string) (ImportJob, error) {
	if s == nil || s.imports == nil || s.writer == nil {
		return ImportJob{}, errors.New("question bank import commit not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, err
	}
	if job.Status == ImportStatusCommitted {
		return job, nil
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	return s.commitReadyJob(ctx, jobID)
}

func (s *ImportService) ReviewItems(ctx context.Context, jobID string, itemIDs []string, reviewStatus string) (ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, nil, errors.New("question bank import review not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, nil, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	if reviewStatus != ImportReviewStatusAccepted && reviewStatus != ImportReviewStatusRejected {
		return ImportJob{}, nil, fmt.Errorf("unsupported import review_status %q", reviewStatus)
	}
	if len(itemIDs) == 0 {
		return ImportJob{}, nil, errors.New("import review requires at least one item id")
	}
	if err := s.imports.UpdateItemReviews(ctx, jobID, itemIDs, reviewStatus); err != nil {
		return ImportJob{}, nil, err
	}
	items, err := s.imports.ListItems(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	return job, items, nil
}

func (s *ImportService) ReviewAllValidItems(ctx context.Context, jobID string, reviewStatus string, completeOnly bool) (ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, nil, errors.New("question bank import review not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, nil, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	if reviewStatus != ImportReviewStatusAccepted && reviewStatus != ImportReviewStatusRejected {
		return ImportJob{}, nil, fmt.Errorf("unsupported import review_status %q", reviewStatus)
	}
	items, err := s.imports.ListItems(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	var ids []string
	for _, item := range items {
		if item.Status != ImportItemStatusValid {
			continue
		}
		if completeOnly && !importItemHasCompleteReviewFields(item.Item) {
			continue
		}
		ids = append(ids, item.ID)
	}
	if len(ids) > 0 {
		if err := s.imports.UpdateItemReviews(ctx, jobID, ids, reviewStatus); err != nil {
			return ImportJob{}, nil, err
		}
		items, err = s.imports.ListItems(ctx, jobID)
		if err != nil {
			return ImportJob{}, nil, err
		}
	}
	return job, items, nil
}

func (s *ImportService) commitReadyJob(ctx context.Context, jobID string) (ImportJob, error) {
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, err
	}
	if job.Status == ImportStatusCommitted {
		return job, nil
	}
	if job.Status != ImportStatusReady && job.Status != ImportStatusQueued && job.Status != ImportStatusCommitting {
		return ImportJob{}, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	job.Status = ImportStatusCommitting
	job, _ = s.imports.UpdateJob(ctx, job)

	importItems, err := s.imports.ListItems(ctx, job.ID)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	var items []Item
	var updated []ImportItem
	for _, item := range importItems {
		if item.Status != ImportItemStatusValid || !importItemAccepted(item) {
			continue
		}
		items = append(items, item.Item)
		item.Status = ImportItemStatusImported
		item.UpdatedAt = time.Now().UTC()
		updated = append(updated, item)
	}
	if err := s.writer.Upsert(ctx, items); err != nil {
		return s.failJob(ctx, job, err)
	}
	if err := s.embedCommittedItems(ctx, items); err != nil {
		return s.failJob(ctx, job, err)
	}
	if err := s.imports.UpdateItems(ctx, updated); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.Status = ImportStatusCommitted
	job.ImportedItems = len(items)
	return s.imports.UpdateJob(ctx, job)
}

func (s *ImportService) embedCommittedItems(ctx context.Context, items []Item) error {
	if s.embedder == nil || len(items) == 0 {
		return nil
	}
	writer, ok := s.writer.(EmbeddingWriter)
	if !ok {
		return nil
	}
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = embedText(item)
	}
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed imported questions: %w", err)
	}
	if len(vectors) != len(items) {
		return fmt.Errorf("embed imported questions: got %d vectors, want %d", len(vectors), len(items))
	}
	out := make([]ItemEmbedding, 0, len(items))
	for i, item := range items {
		if len(vectors[i]) != s.embedder.Dimension() {
			return fmt.Errorf("%w: question %s vector dim %d, want %d", embedding.ErrDimensionMismatch, item.ID, len(vectors[i]), s.embedder.Dimension())
		}
		out = append(out, ItemEmbedding{
			ID:     item.ID,
			Vector: vectors[i],
			Model:  s.embedder.Name(),
		})
	}
	return writer.UpsertEmbeddings(ctx, out)
}

func importItemAccepted(item ImportItem) bool {
	return item.ReviewStatus == "" || item.ReviewStatus == ImportReviewStatusAccepted
}

func normalizedImportReviewStatus(status string) string {
	if status == ImportReviewStatusRejected {
		return ImportReviewStatusRejected
	}
	return ImportReviewStatusAccepted
}

func importItemHasCompleteReviewFields(item Item) bool {
	return strings.TrimSpace(item.Content) != "" &&
		strings.TrimSpace(item.SkillCategory) != "" &&
		item.Difficulty > 0 &&
		len(item.Tags) > 0 &&
		len(item.ExpectedPoints) > 0 &&
		len(item.Rubric) > 0 &&
		strings.TrimSpace(item.SampleAnswer) != "" &&
		len(item.FollowUpHints) > 0
}
