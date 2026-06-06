package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

type pgSessionExec func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)

func (s *PGSessionStore) Save(ctx context.Context, sess *domain.Session) error {
	if s == nil || s.Pool == nil {
		return fmt.Errorf("pg session store: pool not initialized")
	}
	return s.saveWithExec(ctx, sess, s.Pool.Exec)
}

func (s *PGSessionStore) saveWithExec(ctx context.Context, sess *domain.Session, exec pgSessionExec) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("pg session store: session id required")
	}
	if exec == nil {
		return fmt.Errorf("pg session store: exec required")
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
	tag, err := exec(ctx, pgSessionUpsertSQL,
		sess.ID, sess.UserID, string(sess.Status), sess.CurrentNode, string(raw), pgInterval(ttl), sess.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %q", ErrStaleSessionWrite, sess.ID)
	}
	return nil
}

const pgSessionUpsertSQL = `
INSERT INTO sessions (id, user_id, status, current_node, state_json, expires_at, updated_at)
VALUES ($1, $2, $3, $4, $5::jsonb, now() + $6::interval, $7)
ON CONFLICT (id) DO UPDATE SET
	user_id = EXCLUDED.user_id,
	status = EXCLUDED.status,
	current_node = EXCLUDED.current_node,
	state_json = EXCLUDED.state_json,
	expires_at = EXCLUDED.expires_at,
	updated_at = EXCLUDED.updated_at
WHERE COALESCE((sessions.state_json->>'updated_at')::timestamptz, sessions.updated_at) <= EXCLUDED.updated_at`

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
	err := s.Pool.QueryRow(ctx, `SELECT state_json FROM sessions WHERE id = $1`, id).Scan(&raw)
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
	sess.MigrateLegacyState()
	return &sess, nil
}

func (s *PGSessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.Session, error) {
	if s == nil || s.Pool == nil {
		return nil, fmt.Errorf("pg session store: pool not initialized")
	}
	limit = normalizeSessionListLimit(limit)
	rows, err := s.Pool.Query(ctx, `
SELECT state_json
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
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		var sess domain.Session
		if err := json.Unmarshal(raw, &sess); err != nil {
			return nil, fmt.Errorf("unmarshal session: %w", err)
		}
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
