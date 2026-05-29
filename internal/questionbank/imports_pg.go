package questionbank

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGImportStore struct {
	Pool *pgxpool.Pool
}

func NewPGImportStore(pool *pgxpool.Pool) *PGImportStore {
	return &PGImportStore{Pool: pool}
}

func (s *PGImportStore) CreateJob(ctx context.Context, job ImportJob) (ImportJob, error) {
	if s == nil || s.Pool == nil {
		return ImportJob{}, errorsNotConfigured()
	}
	meta, _ := json.Marshal(job.Metadata)
	var metaRaw []byte
	err := s.Pool.QueryRow(ctx, `
INSERT INTO question_bank_import_jobs (
    id, source_type, filename, status, owner_id, lease_until, total_chunks, total_items, valid_items,
    invalid_items, imported_items, error, metadata, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15)
RETURNING id, source_type, filename, status, owner_id, COALESCE(lease_until, '0001-01-01T00:00:00Z'::timestamptz), total_chunks, total_items, valid_items,
          invalid_items, imported_items, error, metadata, created_at, updated_at
`, job.ID, job.SourceType, job.Filename, job.Status, job.OwnerID, nilTime(job.LeaseUntil), job.TotalChunks, job.TotalItems, job.ValidItems,
		job.InvalidItems, job.ImportedItems, job.Error, string(meta), job.CreatedAt, job.UpdatedAt).Scan(
		&job.ID, &job.SourceType, &job.Filename, &job.Status, &job.OwnerID, &job.LeaseUntil, &job.TotalChunks, &job.TotalItems,
		&job.ValidItems, &job.InvalidItems, &job.ImportedItems, &job.Error, &metaRaw,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return ImportJob{}, fmt.Errorf("create question bank import job: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &job.Metadata)
	return job, nil
}

func (s *PGImportStore) UpdateJob(ctx context.Context, job ImportJob) (ImportJob, error) {
	meta, _ := json.Marshal(job.Metadata)
	var metaRaw []byte
	err := s.Pool.QueryRow(ctx, `
UPDATE question_bank_import_jobs
SET status=$2, total_chunks=$3, total_items=$4, valid_items=$5, invalid_items=$6,
    imported_items=$7, error=$8, metadata=$9::jsonb, updated_at=now()
WHERE id=$1
RETURNING id, source_type, filename, status, owner_id, COALESCE(lease_until, '0001-01-01T00:00:00Z'::timestamptz), total_chunks, total_items, valid_items,
          invalid_items, imported_items, error, metadata, created_at, updated_at
`, job.ID, job.Status, job.TotalChunks, job.TotalItems, job.ValidItems, job.InvalidItems,
		job.ImportedItems, job.Error, string(meta)).Scan(
		&job.ID, &job.SourceType, &job.Filename, &job.Status, &job.OwnerID, &job.LeaseUntil, &job.TotalChunks, &job.TotalItems,
		&job.ValidItems, &job.InvalidItems, &job.ImportedItems, &job.Error, &metaRaw,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return ImportJob{}, ErrImportNotFound
	}
	if err != nil {
		return ImportJob{}, fmt.Errorf("update question bank import job: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &job.Metadata)
	return job, nil
}

func (s *PGImportStore) GetJob(ctx context.Context, id string) (ImportJob, error) {
	var job ImportJob
	var metaRaw []byte
	err := s.Pool.QueryRow(ctx, `
SELECT id, source_type, filename, status, owner_id, COALESCE(lease_until, '0001-01-01T00:00:00Z'::timestamptz), total_chunks, total_items, valid_items,
       invalid_items, imported_items, error, metadata, created_at, updated_at
FROM question_bank_import_jobs
WHERE id=$1
`, id).Scan(
		&job.ID, &job.SourceType, &job.Filename, &job.Status, &job.OwnerID, &job.LeaseUntil, &job.TotalChunks, &job.TotalItems,
		&job.ValidItems, &job.InvalidItems, &job.ImportedItems, &job.Error, &metaRaw,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return ImportJob{}, ErrImportNotFound
	}
	if err != nil {
		return ImportJob{}, fmt.Errorf("get question bank import job: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &job.Metadata)
	return job, nil
}

func (s *PGImportStore) ListJobs(ctx context.Context) ([]ImportJob, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT id, source_type, filename, status, owner_id, COALESCE(lease_until, '0001-01-01T00:00:00Z'::timestamptz), total_chunks, total_items, valid_items,
       invalid_items, imported_items, error, metadata, created_at, updated_at
FROM question_bank_import_jobs
ORDER BY created_at DESC
LIMIT 50
`)
	if err != nil {
		return nil, fmt.Errorf("list question bank import jobs: %w", err)
	}
	defer rows.Close()
	var jobs []ImportJob
	for rows.Next() {
		var job ImportJob
		var metaRaw []byte
		if err := rows.Scan(
			&job.ID, &job.SourceType, &job.Filename, &job.Status, &job.OwnerID, &job.LeaseUntil, &job.TotalChunks, &job.TotalItems,
			&job.ValidItems, &job.InvalidItems, &job.ImportedItems, &job.Error, &metaRaw,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metaRaw, &job.Metadata)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *PGImportStore) AddChunks(ctx context.Context, chunks []ImportChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, chunk := range chunks {
		meta, _ := json.Marshal(chunk.Metadata)
		batch.Queue(`
INSERT INTO question_bank_import_chunks (id, job_id, chunk_index, content, metadata, created_at)
VALUES ($1,$2,$3,$4,$5::jsonb,$6)
ON CONFLICT (id) DO NOTHING
`, chunk.ID, chunk.JobID, chunk.Index, chunk.Content, string(meta), chunk.CreatedAt)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range chunks {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("add question bank import chunks: %w", err)
		}
	}
	return nil
}

func (s *PGImportStore) ListChunks(ctx context.Context, jobID string) ([]ImportChunk, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT id, job_id, chunk_index, content, metadata, created_at
FROM question_bank_import_chunks
WHERE job_id=$1
ORDER BY chunk_index
`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list question bank import chunks: %w", err)
	}
	defer rows.Close()
	var chunks []ImportChunk
	for rows.Next() {
		var chunk ImportChunk
		var meta []byte
		if err := rows.Scan(&chunk.ID, &chunk.JobID, &chunk.Index, &chunk.Content, &meta, &chunk.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meta, &chunk.Metadata)
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *PGImportStore) AddItems(ctx context.Context, items []ImportItem) error {
	if len(items) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, item := range items {
		itemJSON, _ := json.Marshal(item.Item)
		rawJSON := itemJSON
		if item.OriginalItem != nil {
			rawJSON, _ = json.Marshal(item.OriginalItem)
		}
		batch.Queue(`
INSERT INTO question_bank_import_items (
    id, job_id, chunk_id, question_id, status, review_status, item_json, errors, raw_json, created_at, updated_at
) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7::jsonb,$8,$9::jsonb,$10,$11)
ON CONFLICT (id) DO UPDATE SET
    status=EXCLUDED.status,
    review_status=EXCLUDED.review_status,
    item_json=EXCLUDED.item_json,
    errors=EXCLUDED.errors,
    raw_json=EXCLUDED.raw_json,
    updated_at=now()
`, item.ID, item.JobID, item.ChunkID, item.QuestionID, item.Status, normalizedImportReviewStatus(item.ReviewStatus), string(itemJSON), item.Errors, string(rawJSON), item.CreatedAt, item.UpdatedAt)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range items {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("add question bank import items: %w", err)
		}
	}
	return nil
}

func (s *PGImportStore) ListItems(ctx context.Context, jobID string) ([]ImportItem, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT id, job_id, COALESCE(chunk_id, ''), question_id, status, COALESCE(review_status, 'accepted'), item_json, raw_json, errors, created_at, updated_at
FROM question_bank_import_items
WHERE job_id=$1
ORDER BY created_at, id
`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list question bank import items: %w", err)
	}
	defer rows.Close()
	var items []ImportItem
	for rows.Next() {
		var item ImportItem
		var itemJSON []byte
		var rawJSON []byte
		if err := rows.Scan(&item.ID, &item.JobID, &item.ChunkID, &item.QuestionID, &item.Status, &item.ReviewStatus, &itemJSON, &rawJSON, &item.Errors, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(itemJSON, &item.Item)
		if len(rawJSON) > 0 && string(rawJSON) != "{}" {
			var original Item
			if err := json.Unmarshal(rawJSON, &original); err == nil {
				item.OriginalItem = &original
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PGImportStore) UpdateItems(ctx context.Context, items []ImportItem) error {
	if len(items) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, item := range items {
		itemJSON, _ := json.Marshal(item.Item)
		batch.Queue(`
UPDATE question_bank_import_items
SET status=$2, review_status=$3, item_json=$4::jsonb, errors=$5, updated_at=now()
WHERE id=$1
`, item.ID, item.Status, normalizedImportReviewStatus(item.ReviewStatus), string(itemJSON), item.Errors)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range items {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("update question bank import items: %w", err)
		}
	}
	return nil
}

func (s *PGImportStore) UpdateItemReviews(ctx context.Context, jobID string, itemIDs []string, reviewStatus string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `
UPDATE question_bank_import_items
SET review_status=$3, updated_at=now()
WHERE job_id=$1 AND id=ANY($2) AND status='valid'
`, jobID, itemIDs, normalizedImportReviewStatus(reviewStatus))
	if err != nil {
		return fmt.Errorf("update question bank import item reviews: %w", err)
	}
	return nil
}

func (s *PGImportStore) ResetJobData(ctx context.Context, jobID string) error {
	if s == nil || s.Pool == nil {
		return errorsNotConfigured()
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reset question bank import job: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM question_bank_import_items WHERE job_id=$1`, jobID); err != nil {
		return fmt.Errorf("reset question bank import items: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM question_bank_import_chunks WHERE job_id=$1`, jobID); err != nil {
		return fmt.Errorf("reset question bank import chunks: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reset question bank import job: %w", err)
	}
	return nil
}

func (s *PGImportStore) TryAcquireJob(ctx context.Context, jobID, ownerID string, leaseFor time.Duration) (ImportJob, bool, error) {
	if s == nil || s.Pool == nil {
		return ImportJob{}, false, errorsNotConfigured()
	}
	if leaseFor <= 0 {
		leaseFor = 2 * time.Minute
	}
	var job ImportJob
	var metaRaw []byte
	err := s.Pool.QueryRow(ctx, `
UPDATE question_bank_import_jobs
SET owner_id=$2,
    lease_until=now() + ($3::text)::interval,
    updated_at=now()
WHERE id=$1
  AND (owner_id = '' OR owner_id = $2 OR lease_until IS NULL OR lease_until < now())
RETURNING id, source_type, filename, status, owner_id,
          COALESCE(lease_until, '0001-01-01T00:00:00Z'::timestamptz),
          total_chunks, total_items, valid_items, invalid_items, imported_items,
          error, metadata, created_at, updated_at
`, jobID, ownerID, fmt.Sprintf("%f seconds", leaseFor.Seconds())).Scan(
		&job.ID, &job.SourceType, &job.Filename, &job.Status, &job.OwnerID, &job.LeaseUntil,
		&job.TotalChunks, &job.TotalItems, &job.ValidItems, &job.InvalidItems, &job.ImportedItems,
		&job.Error, &metaRaw, &job.CreatedAt, &job.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		_, getErr := s.GetJob(ctx, jobID)
		if getErr != nil {
			return ImportJob{}, false, getErr
		}
		return ImportJob{}, false, nil
	}
	if err != nil {
		return ImportJob{}, false, fmt.Errorf("acquire question bank import job lease: %w", err)
	}
	_ = json.Unmarshal(metaRaw, &job.Metadata)
	return job, true, nil
}

func nilTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func errorsNotConfigured() error {
	return fmt.Errorf("question bank import postgres store not configured")
}
