package questionbank

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGGenerationJobStore struct {
	Pool *pgxpool.Pool
}

func NewPGGenerationJobStore(pool *pgxpool.Pool) *PGGenerationJobStore {
	return &PGGenerationJobStore{Pool: pool}
}

func (s *PGGenerationJobStore) Create(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	if s == nil || s.Pool == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return GenerationJob{}, fmt.Errorf("marshal generation job: %w", err)
	}
	var out []byte
	err = s.Pool.QueryRow(ctx, `
INSERT INTO question_bank_generation_jobs (
    id, status, source_job_id, request_json, job_json, created_at, updated_at
) VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6,$7)
RETURNING job_json
`, job.ID, job.Status, job.Request.SourceJobID, marshalGenerationRequest(job.Request), string(payload), job.CreatedAt, job.UpdatedAt).Scan(&out)
	if err != nil {
		return GenerationJob{}, fmt.Errorf("create question bank generation job: %w", err)
	}
	return decodeGenerationJob(out)
}

func (s *PGGenerationJobStore) Update(ctx context.Context, job GenerationJob) (GenerationJob, error) {
	if s == nil || s.Pool == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return GenerationJob{}, fmt.Errorf("marshal generation job: %w", err)
	}
	var out []byte
	err = s.Pool.QueryRow(ctx, `
UPDATE question_bank_generation_jobs
SET status=$2, source_job_id=$3, request_json=$4::jsonb, job_json=$5::jsonb, updated_at=$6
WHERE id=$1
RETURNING job_json
`, job.ID, job.Status, job.Request.SourceJobID, marshalGenerationRequest(job.Request), string(payload), job.UpdatedAt).Scan(&out)
	if err == pgx.ErrNoRows {
		return GenerationJob{}, ErrImportNotFound
	}
	if err != nil {
		return GenerationJob{}, fmt.Errorf("update question bank generation job: %w", err)
	}
	return decodeGenerationJob(out)
}

func (s *PGGenerationJobStore) Get(ctx context.Context, id string) (GenerationJob, error) {
	if s == nil || s.Pool == nil {
		return GenerationJob{}, errorsNotConfigured()
	}
	var payload []byte
	err := s.Pool.QueryRow(ctx, `
SELECT job_json
FROM question_bank_generation_jobs
WHERE id=$1
`, id).Scan(&payload)
	if err == pgx.ErrNoRows {
		return GenerationJob{}, ErrImportNotFound
	}
	if err != nil {
		return GenerationJob{}, fmt.Errorf("get question bank generation job: %w", err)
	}
	return decodeGenerationJob(payload)
}

func marshalGenerationRequest(req GenerationRequest) string {
	raw, _ := json.Marshal(req)
	return string(raw)
}

func decodeGenerationJob(raw []byte) (GenerationJob, error) {
	var job GenerationJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return GenerationJob{}, fmt.Errorf("decode generation job: %w", err)
	}
	return cloneGenerationJob(job), nil
}
