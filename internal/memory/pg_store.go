package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	Pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) Store {
	return &PGStore{Pool: pool}
}

type pgMemoryQueryRow func(ctx context.Context, sql string, args ...any) pgMemoryRow

type pgMemoryRow interface {
	Scan(dest ...any) error
}

type pgMemoryExec func(ctx context.Context, sql string, args ...any) error

func (s *PGStore) GetUserMemory(ctx context.Context, userID string) (*UserMemory, error) {
	if s == nil || s.Pool == nil {
		return nil, fmt.Errorf("%w: pg memory store pool not initialized", ErrUserMemoryNotFound)
	}
	return s.getWithQueryRow(ctx, userID, func(ctx context.Context, sql string, args ...any) pgMemoryRow {
		return s.Pool.QueryRow(ctx, sql, args...)
	})
}

func (s *PGStore) getWithQueryRow(ctx context.Context, userID string, queryRow pgMemoryQueryRow) (*UserMemory, error) {
	userID = strings.TrimSpace(userID)
	if queryRow == nil {
		return nil, fmt.Errorf("%w: query row is required", ErrUserMemoryNotFound)
	}
	var raw []byte
	var updatedAt time.Time
	err := queryRow(ctx, pgMemorySelectSQL, userID).Scan(&raw, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrUserMemoryNotFound, userID)
		}
		return nil, fmt.Errorf("select user memory: %w", err)
	}
	var mem UserMemory
	if err := json.Unmarshal(raw, &mem); err != nil {
		return nil, fmt.Errorf("unmarshal user memory: %w", err)
	}
	mem.UserID = userID
	mem.UpdatedAt = updatedAt
	return cloneUserMemory(&mem), nil
}

const pgMemorySelectSQL = `
SELECT memory_json, updated_at
FROM user_memory
WHERE user_id = $1`

func (s *PGStore) UpsertUserMemory(ctx context.Context, memory *UserMemory) error {
	if s == nil || s.Pool == nil {
		return fmt.Errorf("%w: pg memory store pool not initialized", ErrInvalidMemoryInput)
	}
	return s.upsertWithExec(ctx, memory, func(ctx context.Context, sql string, args ...any) error {
		_, err := s.Pool.Exec(ctx, sql, args...)
		return err
	})
}

func (s *PGStore) upsertWithExec(ctx context.Context, memory *UserMemory, exec pgMemoryExec) error {
	if memory == nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	userID := strings.TrimSpace(memory.UserID)
	if userID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	if exec == nil {
		return fmt.Errorf("%w: exec is required", ErrInvalidMemoryInput)
	}
	cloned := cloneUserMemory(memory)
	cloned.UserID = userID
	raw, err := json.Marshal(cloned)
	if err != nil {
		return fmt.Errorf("marshal user memory: %w", err)
	}
	if err := exec(ctx, pgMemoryUpsertSQL, userID, raw, cloned.UpdatedAt); err != nil {
		return fmt.Errorf("upsert user memory: %w", err)
	}
	return nil
}

const pgMemoryUpsertSQL = `
INSERT INTO user_memory (user_id, memory_json, updated_at)
VALUES ($1, $2::jsonb, $3)
ON CONFLICT (user_id) DO UPDATE SET
	memory_json = EXCLUDED.memory_json,
	updated_at = EXCLUDED.updated_at`
