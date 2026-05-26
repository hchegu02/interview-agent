package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"interview-agent/internal/domain"
)

const defaultRedisSessionSnapshotTTL = 24 * time.Hour

type RedisSessionCoordinatorOptions struct {
	Addr        string
	Username    string
	Password    string
	DB          int
	TLS         bool
	KeyPrefix   string
	DialTimeout time.Duration
}

type RedisSessionCoordinator struct {
	opts   RedisSessionCoordinatorOptions
	dialer redisDialer
}

func ParseRedisSessionCoordinatorOptions(rawURL string) (RedisSessionCoordinatorOptions, error) {
	if rawURL == "" {
		return RedisSessionCoordinatorOptions{}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return RedisSessionCoordinatorOptions{}, fmt.Errorf("parse redis url: %w", err)
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return RedisSessionCoordinatorOptions{}, fmt.Errorf("unsupported redis scheme %q", u.Scheme)
	}
	opts := RedisSessionCoordinatorOptions{
		Addr:     u.Host,
		Username: u.User.Username(),
		TLS:      u.Scheme == "rediss",
	}
	if pw, ok := u.User.Password(); ok {
		opts.Password = pw
	}
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		db, err := strconv.Atoi(path)
		if err != nil {
			return RedisSessionCoordinatorOptions{}, fmt.Errorf("parse redis db: %w", err)
		}
		opts.DB = db
	}
	return opts, nil
}

func NewRedisSessionCoordinator(opts RedisSessionCoordinatorOptions) (*RedisSessionCoordinator, error) {
	opts = opts.withDefaults()
	if opts.Addr == "" {
		return nil, fmt.Errorf("redis session coordinator: addr required")
	}
	c := &RedisSessionCoordinator{opts: opts}
	c.dialer = c.dial
	return c, nil
}

func (o RedisSessionCoordinatorOptions) withDefaults() RedisSessionCoordinatorOptions {
	if o.KeyPrefix == "" {
		o.KeyPrefix = "interview:session"
	}
	if o.DialTimeout <= 0 {
		o.DialTimeout = 5 * time.Second
	}
	return o
}

func (c *RedisSessionCoordinator) SaveSnapshot(ctx context.Context, sess *domain.Session, ttl time.Duration) error {
	if c == nil {
		return fmt.Errorf("redis session coordinator: nil coordinator")
	}
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("redis session coordinator: session id required")
	}
	if ttl <= 0 {
		ttl = defaultRedisSessionSnapshotTTL
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session snapshot: %w", err)
	}
	conn, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = redisDoString(conn, "SET", c.snapshotKey(sess.ID), string(raw), "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		return fmt.Errorf("save session snapshot: %w", err)
	}
	return nil
}

func (c *RedisSessionCoordinator) LoadSnapshot(ctx context.Context, sessionID string) (*domain.Session, error) {
	if c == nil {
		return nil, fmt.Errorf("redis session coordinator: nil coordinator")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("redis session coordinator: session id required")
	}
	conn, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	value, err := redisDo(conn, "GET", c.snapshotKey(sessionID))
	if err != nil {
		return nil, fmt.Errorf("load session snapshot: %w", err)
	}
	raw, ok := value.(string)
	if !ok || raw == "" {
		return nil, fmt.Errorf("session snapshot %q not found", sessionID)
	}
	var sess domain.Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session snapshot: %w", err)
	}
	sess.MigrateLegacyState()
	return &sess, nil
}

func (c *RedisSessionCoordinator) DeleteSnapshot(ctx context.Context, sessionID string) error {
	if c == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	conn, err := c.open(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = redisDo(conn, "DEL", c.snapshotKey(sessionID))
	return err
}

func (c *RedisSessionCoordinator) AcquireLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	if err := validateLeaseInput(sessionID, ownerID, ttl); err != nil {
		return false, err
	}
	conn, err := c.open(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	value, err := redisDo(conn, "SET", c.leaseKey(sessionID), ownerID, "NX", "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		return false, fmt.Errorf("acquire session lease: %w", err)
	}
	return value == "OK", nil
}

func (c *RedisSessionCoordinator) RenewLease(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (bool, error) {
	if err := validateLeaseInput(sessionID, ownerID, ttl); err != nil {
		return false, err
	}
	return c.evalOwnerScript(ctx, sessionID, ownerID, renewLeaseScript, strconv.FormatInt(ttl.Milliseconds(), 10))
}

func (c *RedisSessionCoordinator) ReleaseLease(ctx context.Context, sessionID, ownerID string) (bool, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(ownerID) == "" {
		return false, fmt.Errorf("redis session coordinator: session id and owner id required")
	}
	return c.evalOwnerScript(ctx, sessionID, ownerID, releaseLeaseScript)
}

func (c *RedisSessionCoordinator) evalOwnerScript(ctx context.Context, sessionID, ownerID, script string, extraArgs ...string) (bool, error) {
	conn, err := c.open(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	args := []string{"EVAL", script, "1", c.leaseKey(sessionID), ownerID}
	args = append(args, extraArgs...)
	value, err := redisDo(conn, args...)
	if err != nil {
		return false, fmt.Errorf("eval session lease script: %w", err)
	}
	n, ok := value.(int64)
	return ok && n == 1, nil
}

func validateLeaseInput(sessionID, ownerID string, ttl time.Duration) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(ownerID) == "" {
		return fmt.Errorf("redis session coordinator: session id and owner id required")
	}
	if ttl <= 0 {
		return fmt.Errorf("redis session coordinator: lease ttl must be positive")
	}
	return nil
}

func (c *RedisSessionCoordinator) snapshotKey(sessionID string) string {
	return c.opts.KeyPrefix + ":snapshot:" + sessionID
}

func (c *RedisSessionCoordinator) leaseKey(sessionID string) string {
	return c.opts.KeyPrefix + ":lease:" + sessionID
}

func (c *RedisSessionCoordinator) open(ctx context.Context) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return nil, fmt.Errorf("redis session coordinator: nil coordinator")
	}
	if c.dialer == nil {
		c.dialer = c.dial
	}
	conn, err := c.dialer(ctx)
	if err != nil {
		return nil, err
	}
	if c.opts.Password != "" {
		args := []string{"AUTH", c.opts.Password}
		if c.opts.Username != "" {
			args = []string{"AUTH", c.opts.Username, c.opts.Password}
		}
		if _, err := redisDoString(conn, args...); err != nil {
			conn.Close()
			return nil, err
		}
	}
	if c.opts.DB > 0 {
		if _, err := redisDoString(conn, "SELECT", strconv.Itoa(c.opts.DB)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (c *RedisSessionCoordinator) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: c.opts.DialTimeout}
	if c.opts.TLS {
		return tls.DialWithDialer(&d, "tcp", c.opts.Addr, &tls.Config{MinVersion: tls.VersionTLS12})
	}
	return d.DialContext(ctx, "tcp", c.opts.Addr)
}

const renewLeaseScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) else return 0 end`

const releaseLeaseScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
