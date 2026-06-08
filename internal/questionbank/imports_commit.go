package questionbank

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/embedding"
)

type importCommitSummary struct {
	Matched         int            `json:"matched"`
	Imported        int            `json:"imported"`
	Skipped         int            `json:"skipped"`
	EmbeddingSynced int            `json:"embedding_synced"`
	EmbeddingFailed int            `json:"embedding_failed"`
	FailureReasons  map[string]int `json:"failure_reasons"`
	EmbeddingErrors []string       `json:"embedding_errors,omitempty"`
}

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
	summary := newImportCommitSummary(0)

	// 只导入“有效且发布策略允许”的 item。无 Agent 审核状态的旧导入保持兼容；
	// Agent 标记为 needs_human_review/rejected 的题不能静默进入正式题库。
	importItems, err := s.imports.ListItems(ctx, job.ID)
	if err != nil {
		return s.failCommitJob(ctx, job, summary, "list_items_failed", err)
	}
	summary.Matched = len(importItems)
	existingContentKeys, err := activeQuestionContentKeys(ctx, s.writer)
	if err != nil {
		return s.failCommitJob(ctx, job, summary, "active_content_read_failed", err)
	}
	var items []Item
	var updated []ImportItem
	seenContentKeys := map[string]struct{}{}
	for _, item := range importItems {
		if item.Status != ImportItemStatusValid {
			summary.recordSkip(importItemStatusSkipReason(item.Status))
			continue
		}
		if !importItemAccepted(item) {
			summary.recordSkip(importItemReviewSkipReason(item))
			continue
		}
		key := questionContentDedupeKey(item.Item.Content)
		if key != "" {
			if existingID, ok := existingContentKeys[key]; ok && existingID != item.Item.ID {
				item.AgentReviewStatus = ImportAgentReviewRejected
				item.AgentReviewReason = qualityFlagDuplicateExistingContent
				item.UpdatedAt = time.Now().UTC()
				updated = append(updated, item)
				summary.recordSkip(qualityFlagDuplicateExistingContent)
				continue
			}
			if _, ok := seenContentKeys[key]; ok {
				item.AgentReviewStatus = ImportAgentReviewRejected
				item.AgentReviewReason = qualityFlagDuplicateContent
				item.UpdatedAt = time.Now().UTC()
				updated = append(updated, item)
				summary.recordSkip(qualityFlagDuplicateContent)
				continue
			}
			seenContentKeys[key] = struct{}{}
		}
		if quality := EvaluateQuestionContentQuality(item.Item.Content); quality.HighRisk {
			item.AgentReviewStatus = ImportAgentReviewRejected
			item.AgentReviewReason = strings.Join(quality.Flags, ",")
			item.UpdatedAt = time.Now().UTC()
			updated = append(updated, item)
			summary.recordSkip(quality.Flags...)
			continue
		}
		items = append(items, item.Item)
		item.Status = ImportItemStatusImported
		item.UpdatedAt = time.Now().UTC()
		updated = append(updated, item)
	}
	if err := s.writer.Upsert(ctx, items); err != nil {
		return s.failCommitJob(ctx, job, summary, "question_upsert_failed", err)
	}
	embeddingSynced, embeddingFailed, embeddingErrs := s.embedCommittedItems(ctx, items)
	summary.EmbeddingSynced = embeddingSynced
	summary.EmbeddingFailed = embeddingFailed
	for _, errText := range embeddingErrs {
		summary.EmbeddingErrors = append(summary.EmbeddingErrors, errText)
	}
	if embeddingFailed > 0 {
		summary.FailureReasons["embedding_failed"] += embeddingFailed
	}
	if err := s.imports.UpdateItems(ctx, updated); err != nil {
		summary.Imported = len(items)
		return s.keepCommitRecoverable(ctx, job, summary, "import_item_update_failed", err)
	}
	job.Status = ImportStatusCommitted
	job.ImportedItems = len(items)
	job.Error = ""
	summary.Imported = len(items)
	job = attachImportCommitSummary(job, summary)
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

func activeQuestionContentKeys(ctx context.Context, source any) (map[string]string, error) {
	store, ok := source.(Store)
	if !ok {
		return nil, nil
	}
	keys := map[string]string{}
	cursor := ""
	for {
		result, err := store.List(ctx, Filter{Status: "active", Limit: 100, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, item := range result.Items {
			key := questionContentDedupeKey(item.Content)
			if key != "" {
				keys[key] = item.ID
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

func contentKeySet(keys map[string]string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(keys))
	for key := range keys {
		out[key] = struct{}{}
	}
	return out
}

func newImportCommitSummary(matched int) importCommitSummary {
	return importCommitSummary{
		Matched:        matched,
		FailureReasons: map[string]int{},
	}
}

func (s *ImportService) failCommitJob(ctx context.Context, job ImportJob, summary importCommitSummary, reason string, err error) (ImportJob, error) {
	summary.recordFailure(reason)
	job = attachImportCommitSummary(job, summary)
	return s.failJob(ctx, job, err)
}

func (s *ImportService) keepCommitRecoverable(ctx context.Context, job ImportJob, summary importCommitSummary, reason string, err error) (ImportJob, error) {
	summary.recordFailure(reason)
	job = attachImportCommitSummary(job, summary)
	job.Status = ImportStatusCommitting
	job.Error = err.Error()
	updated, updateErr := s.imports.UpdateJob(ctx, job)
	if updateErr != nil {
		return updated, errors.Join(err, updateErr)
	}
	return updated, err
}

func attachImportCommitSummary(job ImportJob, summary importCommitSummary) ImportJob {
	job.Metadata = cloneStringMap(job.Metadata)
	if job.Metadata == nil {
		job.Metadata = map[string]string{}
	}
	raw, _ := json.Marshal(summary)
	job.Metadata[ImportMetadataCommitSummary] = string(raw)
	return job
}

func (s *importCommitSummary) recordFailure(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "commit_failed"
	}
	if s.FailureReasons == nil {
		s.FailureReasons = map[string]int{}
	}
	s.FailureReasons[reason]++
}

func (s *importCommitSummary) recordSkip(reasons ...string) {
	s.Skipped++
	if s.FailureReasons == nil {
		s.FailureReasons = map[string]int{}
	}
	recorded := false
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		s.FailureReasons[reason]++
		recorded = true
	}
	if !recorded {
		s.FailureReasons["skipped"]++
	}
}

func importItemStatusSkipReason(status string) string {
	switch status {
	case ImportItemStatusInvalid:
		return "invalid"
	case ImportItemStatusRejected:
		return "item_rejected"
	case ImportItemStatusImported:
		return "already_imported"
	default:
		if strings.TrimSpace(status) == "" {
			return "missing_status"
		}
		return "status_" + status
	}
}

func importItemReviewSkipReason(item ImportItem) string {
	if normalizedImportReviewStatus(item.ReviewStatus) == ImportReviewStatusRejected {
		return "review_rejected"
	}
	switch item.AgentReviewStatus {
	case ImportAgentReviewRejected:
		if item.AgentReviewReason != "" {
			return item.AgentReviewReason
		}
		return "agent_rejected"
	case ImportAgentReviewNeedsHumanReview:
		return ImportAgentReviewNeedsHumanReview
	default:
		return "not_accepted"
	}
}

func (s *ImportService) embedCommittedItems(ctx context.Context, items []Item) (int, int, []string) {
	if s.embedder == nil || len(items) == 0 {
		return 0, 0, nil
	}
	writer, ok := s.writer.(EmbeddingWriter)
	if !ok {
		return 0, 0, nil
	}
	// 向量写入是提交后的增强步骤：题库 item 先 upsert 成功，再补 embedding。
	// 如果 embedding 失败，正式题库事实保留，并通过 item/job 诊断状态暴露。
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = embedText(item)
	}
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return s.markCommittedEmbeddingsFailed(ctx, items, fmt.Errorf("embed imported questions: %w", err))
	}
	if len(vectors) != len(items) {
		return s.markCommittedEmbeddingsFailed(ctx, items, fmt.Errorf("embed imported questions: got %d vectors, want %d", len(vectors), len(items)))
	}
	out := make([]ItemEmbedding, 0, len(items))
	for i, item := range items {
		if len(vectors[i]) != s.embedder.Dimension() {
			return s.markCommittedEmbeddingsFailed(ctx, items, fmt.Errorf("%w: question %s vector dim %d, want %d", embedding.ErrDimensionMismatch, item.ID, len(vectors[i]), s.embedder.Dimension()))
		}
		out = append(out, ItemEmbedding{
			ID:     item.ID,
			Vector: vectors[i],
			Model:  s.embedder.Name(),
		})
	}
	if err := writer.UpsertEmbeddings(ctx, out); err != nil {
		return s.markCommittedEmbeddingsFailed(ctx, items, err)
	}
	return len(items), 0, nil
}

func (s *ImportService) markCommittedEmbeddingsFailed(ctx context.Context, items []Item, err error) (int, int, []string) {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	errorsOut := []string{err.Error()}
	if failureWriter, ok := s.writer.(EmbeddingFailureWriter); ok {
		if markErr := failureWriter.MarkEmbeddingsFailed(ctx, ids, err); markErr != nil {
			errorsOut = append(errorsOut, markErr.Error())
		}
	}
	return 0, len(items), errorsOut
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
