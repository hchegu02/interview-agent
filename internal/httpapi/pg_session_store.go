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

func (s *PGSessionStore) Save(ctx context.Context, sess *domain.Session) error {
	if s == nil || s.Pool == nil {
		return fmt.Errorf("pg session store: pool not initialized")
	}
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("pg session store: session id required")
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	ttl := s.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_, err = s.Pool.Exec(ctx, `
INSERT INTO sessions (id, user_id, status, current_node, state_json, expires_at)
VALUES ($1, $2, $3, $4, $5::jsonb, now() + $6::interval)
ON CONFLICT (id) DO UPDATE SET
	user_id = EXCLUDED.user_id,
	status = EXCLUDED.status,
	current_node = EXCLUDED.current_node,
	state_json = EXCLUDED.state_json,
	expires_at = EXCLUDED.expires_at`,
		sess.ID, sess.UserID, string(sess.Status), sess.CurrentNode, string(raw), pgInterval(ttl))
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
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

func pgInterval(d time.Duration) string {
	seconds := int64(d.Seconds())
	if seconds <= 0 {
		seconds = int64((24 * time.Hour).Seconds())
	}
	return fmt.Sprintf("%d seconds", seconds)
}
