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
	// Commit 是从“人工审核后的暂存区”写入正式题库。这里先检查状态，
	// 防止 queued/running 的任务被提前导入，也让重复提交保持幂等。
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
	if err := s.ensureNoPendingHumanReview(ctx, job.ID); err != nil {
		return ImportJob{}, err
	}
	return s.commitReadyJob(ctx, jobID)
}

func (s *ImportService) ReviewItems(ctx context.Context, jobID string, itemIDs []string, reviewStatus string) (ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, nil, errors.New("question bank import review not configured")
	}
	// ReviewItems 只改审核状态，不写正式题库。真正写入发生在 Commit，
	// 这样前端可以多次调整 accept/reject，再一次性提交。
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

	// 只导入“有效且发布策略允许”的 item。无 Agent 审核状态的旧导入保持兼容；
	// Agent 标记为 needs_human_review/rejected 的题不能静默进入正式题库。
	importItems, err := s.imports.ListItems(ctx, job.ID)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	existingContentKeys, err := activeQuestionContentKeys(ctx, s.writer)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	var items []Item
	var updated []ImportItem
	seenContentKeys := map[string]struct{}{}
	for _, item := range importItems {
		if item.Status != ImportItemStatusValid || !importItemAccepted(item) {
			continue
		}
		key := questionContentDedupeKey(item.Item.Content)
		if key != "" {
			if _, ok := existingContentKeys[key]; ok {
				item.AgentReviewStatus = ImportAgentReviewRejected
				item.AgentReviewReason = qualityFlagDuplicateExistingContent
				item.UpdatedAt = time.Now().UTC()
				updated = append(updated, item)
				continue
			}
			if _, ok := seenContentKeys[key]; ok {
				item.AgentReviewStatus = ImportAgentReviewRejected
				item.AgentReviewReason = qualityFlagDuplicateContent
				item.UpdatedAt = time.Now().UTC()
				updated = append(updated, item)
				continue
			}
			seenContentKeys[key] = struct{}{}
		}
		if quality := EvaluateQuestionContentQuality(item.Item.Content); quality.HighRisk {
			item.AgentReviewStatus = ImportAgentReviewRejected
			item.AgentReviewReason = strings.Join(quality.Flags, ",")
			item.UpdatedAt = time.Now().UTC()
			updated = append(updated, item)
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

func (s *ImportService) ensureNoPendingHumanReview(ctx context.Context, jobID string) error {
	items, err := s.imports.ListItems(ctx, jobID)
	if err != nil {
		return err
	}
	var pending []string
	for _, item := range items {
		if item.Status != ImportItemStatusValid {
			continue
		}
		if normalizedImportReviewStatus(item.ReviewStatus) != ImportReviewStatusAccepted {
			continue
		}
		if item.AgentReviewStatus == ImportAgentReviewNeedsHumanReview {
			pending = append(pending, item.ID)
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("import job %s requires human review for %d item(s): %s", jobID, len(pending), strings.Join(pending, ","))
	}
	return nil
}

func activeQuestionContentKeys(ctx context.Context, source any) (map[string]struct{}, error) {
	store, ok := source.(Store)
	if !ok {
		return nil, nil
	}
	keys := map[string]struct{}{}
	cursor := ""
	for {
		result, err := store.List(ctx, Filter{Status: "active", Limit: 100, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			key := questionContentDedupeKey(item.Content)
			if key != "" {
				keys[key] = struct{}{}
			}
		}
		if result.NextCursor == "" {
			break
		}
		if result.NextCursor == cursor {
			return nil, fmt.Errorf("question bank active content cursor did not advance: %s", cursor)
		}
		cursor = result.NextCursor
	}
	return keys, nil
}

func (s *ImportService) embedCommittedItems(ctx context.Context, items []Item) error {
	if s.embedder == nil || len(items) == 0 {
		return nil
	}
	writer, ok := s.writer.(EmbeddingWriter)
	if !ok {
		return nil
	}
	// 向量写入是提交后的增强步骤：题库 item 先 upsert 成功，再补 embedding。
	// 如果 embedding 失败，整个 job 标记失败，避免出现“题库有题但检索不到”的隐性坏状态。
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
	if item.AgentReviewStatus != "" && item.AgentReviewStatus != ImportAgentReviewAutoApproved {
		return false
	}
	return item.ReviewStatus == "" || item.ReviewStatus == ImportReviewStatusAccepted
}

func agentReviewStatusAfterHumanReview(current, reviewStatus string) string {
	reviewStatus = normalizedImportReviewStatus(reviewStatus)
	switch reviewStatus {
	case ImportReviewStatusAccepted:
		if current != "" && current != ImportAgentReviewAutoApproved {
			return ImportAgentReviewAutoApproved
		}
	case ImportReviewStatusRejected:
		if current != "" {
			return ImportAgentReviewRejected
		}
	}
	return current
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
