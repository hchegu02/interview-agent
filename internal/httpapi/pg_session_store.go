package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/domain"
)

type PGSessionStore struct {
	Pool *pgxpool.Pool
	TTL  time.Duration
}

func NewPGSessionStore(pool *pgxpool.Pool, ttl time.Duration) *PGSessionStore {
	return &PGSessionStore{Pool: pool, TTL: ttl}
}

type pgSessionQueryRow func(ctx context.Context, sql string, args ...any) pgSessionRow

type pgSessionRow interface {
	Scan(dest ...any) error
}

func (s *PGSessionStore) Save(ctx context.Context, sess *domain.Session) error {
	if s == nil || s.Pool == nil {
		return fmt.Errorf("pg session store: pool not initialized")
	}
	return s.saveWithQueryRow(ctx, sess, func(ctx context.Context, sql string, args ...any) pgSessionRow {
		return s.Pool.QueryRow(ctx, sql, args...)
	})
}

func (s *PGSessionStore) saveWithQueryRow(ctx context.Context, sess *domain.Session, queryRow pgSessionQueryRow) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("pg session store: session id required")
	}
	if queryRow == nil {
		return fmt.Errorf("pg session store: query row required")
	}
	ensureSessionUpdatedAt(sess)
	raw, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	var rowVersion int64
	err = queryRow(ctx, pgSessionUpsertSQL,
		sess.ID, sess.UserID, string(sess.Status), sess.CurrentNode, string(raw), pgInterval(ttl), sess.UpdatedAt, sess.RowVersion).Scan(&rowVersion)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("%w: %q", ErrStaleSessionWrite, sess.ID)
		}
		return fmt.Errorf("upsert session: %w", err)
	}
	sess.RowVersion = rowVersion
	return nil
}

const pgSessionUpsertSQL = `
INSERT INTO sessions (id, user_id, status, current_node, state_json, expires_at, updated_at, row_version)
VALUES ($1, $2, $3, $4, $5::jsonb, now() + $6::interval, $7, 1)
ON CONFLICT (id) DO UPDATE SET
	user_id = EXCLUDED.user_id,
	status = EXCLUDED.status,
	current_node = EXCLUDED.current_node,
	state_json = EXCLUDED.state_json,
	expires_at = EXCLUDED.expires_at,
	updated_at = EXCLUDED.updated_at,
	row_version = sessions.row_version + 1
WHERE sessions.row_version = $8
RETURNING row_version`

func ensureSessionUpdatedAt(sess *domain.Session) {
	if sess == nil || !sess.UpdatedAt.IsZero() {
		return
	}
	now := time.Now()
	sess.UpdatedAt = now
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
}

func (s *PGSessionStore) Get(ctx context.Context, id string) (*domain.Session, error) {
	if s == nil || s.Pool == nil {
		return nil, fmt.Errorf("pg session store: pool not initialized")
	}
	var raw []byte
	var rowVersion int64
	err := s.Pool.QueryRow(ctx, `SELECT state_json, row_version FROM sessions WHERE id = $1`, id).Scan(&raw, &rowVersion)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("%w: %q", ErrSessionNotFound, id)
		}
		return nil, fmt.Errorf("select session: %w", err)
	}
	var sess domain.Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	sess.RowVersion = rowVersion
	sess.MigrateLegacyState()
	return &sess, nil
}

func (s *PGSessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error) {
	if s == nil || s.Pool == nil {
		return nil, fmt.Errorf("pg session store: pool not initialized")
	}
	limit = normalizeSessionListLimit(limit)
	rows, err := s.Pool.Query(ctx, `
SELECT state_json, row_version
FROM sessions
WHERE user_id = $1
ORDER BY updated_at DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		var raw []byte
		var rowVersion int64
		if err := rows.Scan(&raw, &rowVersion); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		var sess domain.Session
		if err := json.Unmarshal(raw, &sess); err != nil {
			return nil, fmt.Errorf("unmarshal session: %w", err)
		}
		sess.RowVersion = rowVersion
		sess.MigrateLegacyState()
		out = append(out, &sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions rows: %w", err)
	}
	return out, nil
}

func (s *PGSessionStore) DeleteForUser(ctx context.Context, id, userID string) error {
	if s == nil || s.Pool == nil {
		return fmt.Errorf("pg session store: pool not initialized")
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %q", ErrSessionNotFound, id)
	}
	return nil
}

func pgInterval(d time.Duration) string {
	seconds := int64(d.Seconds())
	if seconds <= 0 {
		seconds = int64((24 * time.Hour).Seconds())
	}
	return fmt.Sprintf("%d seconds", seconds)
}
