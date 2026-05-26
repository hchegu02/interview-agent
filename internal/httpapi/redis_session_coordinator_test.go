package httpapi

import (
	"context"
	"os"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

func TestParseRedisSessionCoordinatorOptions(t *testing.T) {
	opts, err := ParseRedisSessionCoordinatorOptions("rediss://alice:secret@localhost:6380/2")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Addr != "localhost:6380" {
		t.Fatalf("addr = %q", opts.Addr)
	}
	if opts.Username != "alice" {
		t.Fatalf("username = %q", opts.Username)
	}
	if opts.Password != "secret" {
		t.Fatalf("password = %q", opts.Password)
	}
	if opts.DB != 2 {
		t.Fatalf("db = %d", opts.DB)
	}
	if !opts.TLS {
		t.Fatal("TLS should be true for rediss")
	}
}

func TestRedisSessionCoordinatorRequiresLeaseTTL(t *testing.T) {
	coord, err := NewRedisSessionCoordinator(RedisSessionCoordinatorOptions{Addr: "redis.test:6379"})
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	if _, err := coord.AcquireLease(context.Background(), "s1", "owner", 0); err == nil {
		t.Fatal("expected zero lease ttl to fail")
	}
	if _, err := coord.RenewLease(context.Background(), "s1", "owner", -time.Second); err == nil {
		t.Fatal("expected negative lease ttl to fail")
	}
}

func openRedisSessionCoordinatorForIntegration(t *testing.T) *RedisSessionCoordinator {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run Redis session coordinator integration tests")
	}
	rawURL := os.Getenv("INTERVIEW_REDIS_URL")
	if rawURL == "" {
		rawURL = "redis://localhost:6379/0"
	}
	opts, err := ParseRedisSessionCoordinatorOptions(rawURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	opts.KeyPrefix = "interview:test:session"
	opts.DialTimeout = time.Second
	coord, err := NewRedisSessionCoordinator(opts)
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	return coord
}

func TestRedisSessionCoordinator_IntegrationSnapshotRoundTrip(t *testing.T) {
	coord := openRedisSessionCoordinatorForIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sessionID := "coord-snapshot-" + time.Now().Format("150405.000000000")
	sess := &domain.Session{
		ID:          sessionID,
		UserID:      "u-coord",
		Status:      domain.StatusRunning,
		CurrentNode: "pick_next",
		WorkingMemory: &domain.WorkingMemory{
			Notes: map[string]string{"reflect_topic": "redis"},
		},
	}
	t.Cleanup(func() {
		_ = coord.DeleteSnapshot(context.Background(), sessionID)
	})

	if err := coord.SaveSnapshot(ctx, sess, time.Hour); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	got, err := coord.LoadSnapshot(ctx, sessionID)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if got.ID != sessionID || got.UserID != "u-coord" {
		t.Fatalf("got wrong session: %+v", got)
	}
	if got.WorkingMemory.ReflectTopic != "redis" {
		t.Fatalf("legacy state should migrate, got reflect topic %q", got.WorkingMemory.ReflectTopic)
	}
}

func TestRedisSessionCoordinator_IntegrationLeaseOwnership(t *testing.T) {
	coord := openRedisSessionCoordinatorForIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sessionID := "coord-lease-" + time.Now().Format("150405.000000000")
	t.Cleanup(func() {
		_, _ = coord.ReleaseLease(context.Background(), sessionID, "owner-a")
		_, _ = coord.ReleaseLease(context.Background(), sessionID, "owner-b")
	})

	acquired, err := coord.AcquireLease(ctx, sessionID, "owner-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire owner-a: %v", err)
	}
	if !acquired {
		t.Fatal("owner-a should acquire empty lease")
	}

	acquired, err = coord.AcquireLease(ctx, sessionID, "owner-b", time.Minute)
	if err != nil {
		t.Fatalf("acquire owner-b: %v", err)
	}
	if acquired {
		t.Fatal("owner-b should not acquire lease held by owner-a")
	}

	renewed, err := coord.RenewLease(ctx, sessionID, "owner-b", time.Minute)
	if err != nil {
		t.Fatalf("renew owner-b: %v", err)
	}
	if renewed {
		t.Fatal("owner-b should not renew owner-a lease")
	}

	renewed, err = coord.RenewLease(ctx, sessionID, "owner-a", time.Minute)
	if err != nil {
		t.Fatalf("renew owner-a: %v", err)
	}
	if !renewed {
		t.Fatal("owner-a should renew its lease")
	}

	released, err := coord.ReleaseLease(ctx, sessionID, "owner-b")
	if err != nil {
		t.Fatalf("release owner-b: %v", err)
	}
	if released {
		t.Fatal("owner-b should not release owner-a lease")
	}

	released, err = coord.ReleaseLease(ctx, sessionID, "owner-a")
	if err != nil {
		t.Fatalf("release owner-a: %v", err)
	}
	if !released {
		t.Fatal("owner-a should release its lease")
	}
}
