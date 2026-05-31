package questionbank

import (
	"context"
	"time"
)

func (s *ImportService) stageItems(ctx context.Context, job ImportJob, chunkID string, items []Item) (ImportJob, error) {
	return s.stageItemsWithOriginals(ctx, job, chunkID, items, nil)
}

func (s *ImportService) stageItemsWithOriginals(ctx context.Context, job ImportJob, chunkID string, items []Item, originals []Item) (ImportJob, error) {
	return s.stageItemsWithOriginalsAndProvenance(ctx, job, chunkID, items, originals, nil)
}

func (s *ImportService) stageItemsWithOriginalsAndProvenance(ctx context.Context, job ImportJob, chunkID string, items []Item, originals []Item, provenances []map[string]string) (ImportJob, error) {
	job.Status = ImportStatusValidating
	job, _ = s.imports.UpdateJob(ctx, job)
	staged := make([]ImportItem, 0, len(items))
	for i, item := range items {
		var original *Item
		if i < len(originals) {
			originalItem := cloneItem(originals[i])
			original = &originalItem
		}
		parsedItem := item
		item = normalizeImportedItem(item)
		fieldProvenance := importFieldProvenance(parsedItem, item, original)
		if i < len(provenances) {
			for field, source := range provenances[i] {
				fieldProvenance[field] = source
			}
		}
		errs := validateImportedItem(item)
		status := ImportItemStatusValid
		if len(errs) > 0 {
			status = ImportItemStatusInvalid
		}
		staged = append(staged, ImportItem{
			ID:              job.ID + ":" + item.ID,
			JobID:           job.ID,
			ChunkID:         chunkID,
			QuestionID:      item.ID,
			Status:          status,
			ReviewStatus:    ImportReviewStatusAccepted,
			Item:            item,
			OriginalItem:    original,
			FieldProvenance: fieldProvenance,
			Errors:          errs,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		})
		if status == ImportItemStatusValid {
			job.ValidItems++
		} else {
			job.InvalidItems++
		}
	}
	if err := s.imports.AddItems(ctx, staged); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.TotalItems += len(staged)
	job.Status = ImportStatusReady
	return s.imports.UpdateJob(ctx, job)
}

func (s *ImportService) failJob(ctx context.Context, job ImportJob, err error) (ImportJob, error) {
	job.Status = ImportStatusFailed
	job.Error = err.Error()
	updated, updateErr := s.imports.UpdateJob(ctx, job)
	if updateErr != nil {
		return updated, updateErr
	}
	return updated, err
}
