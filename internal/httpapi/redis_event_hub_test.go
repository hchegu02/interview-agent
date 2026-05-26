package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseRedisEventHubOptions(t *testing.T) {
	opts, err := ParseRedisEventHubOptions("rediss://:secret@localhost:6380/2")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Addr != "localhost:6380" {
		t.Fatalf("addr = %q", opts.Addr)
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

func TestParseRedisEventHubOptions_WithUsername(t *testing.T) {
	opts, err := ParseRedisEventHubOptions("redis://alice:secret@localhost:6379/3")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Username != "alice" {
		t.Fatalf("username = %q", opts.Username)
	}
	if opts.Password != "secret" {
		t.Fatalf("password = %q", opts.Password)
	}
	if opts.DB != 3 {
		t.Fatalf("db = %d", opts.DB)
	}
}

func TestRedisInterviewEventHubOpen_AuthenticatesWithUsername(t *testing.T) {
	hub, err := NewRedisInterviewEventHub(RedisEventHubOptions{
		Addr:     "redis.test:6379",
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	server, client := net.Pipe()
	defer server.Close()
	hub.dialer = func(context.Context) (net.Conn, error) {
		return client, nil
	}

	errCh := make(chan error, 1)
	go func() {
		defer server.Close()
		var raw strings.Builder
		buf := make([]byte, 256)
		n, err := server.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		raw.Write(buf[:n])
		if _, err := io.WriteString(server, "+OK\r\n"); err != nil {
			errCh <- err
			return
		}
		got := raw.String()
		want := "*3\r\n$4\r\nAUTH\r\n$5\r\nalice\r\n$6\r\nsecret\r\n"
		if got != want {
			errCh <- errors.New("unexpected AUTH command")
			return
		}
		errCh <- nil
	}()

	conn, err := hub.open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestEncodeRedisCommand(t *testing.T) {
	got := string(encodeRedisCommand("XADD", "stream", "*", "event", "value"))
	want := "*5\r\n$4\r\nXADD\r\n$6\r\nstream\r\n$1\r\n*\r\n$5\r\nevent\r\n$5\r\nvalue\r\n"
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

func TestReadRedisValue_Array(t *testing.T) {
	raw := "*2\r\n$3\r\nfoo\r\n:42\r\n"
	got, err := readRedisValue(bufio.NewReader(bytes.NewBufferString(raw)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("got = %#v", got)
	}
	if arr[0] != "foo" || arr[1] != int64(42) {
		t.Fatalf("array = %#v", arr)
	}
}

func TestRedisStreamEvents(t *testing.T) {
	value := []any{
		[]any{
			"interview:events:s1",
			[]any{
				[]any{
					"1-0",
					[]any{
						"event",
						`{"type":"session.updated","session_id":"s1"}`,
					},
				},
			},
		},
	}

	events := redisStreamEvents(value)
	if len(events) != 1 {
		t.Fatalf("len = %d, want 1", len(events))
	}
	if events[0].ID != "1-0" {
		t.Fatalf("id = %q", events[0].ID)
	}
	if events[0].Type != interviewEventSessionUpdated {
		t.Fatalf("type = %q", events[0].Type)
	}
	if events[0].SessionID != "s1" {
		t.Fatalf("session_id = %q", events[0].SessionID)
	}
}

func TestRedisInterviewEventHubPublishRecordsError(t *testing.T) {
	hub, err := NewRedisInterviewEventHub(RedisEventHubOptions{Addr: "redis.test:6379"})
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	hub.dialer = func(context.Context) (net.Conn, error) {
		return nil, errors.New("dial failed")
	}

	event := hub.Publish(context.Background(), InterviewEvent{
		Type:      interviewEventSessionUpdated,
		SessionID: "s-publish-error",
	})
	if event.ID != "" {
		t.Fatalf("event id = %q, want empty when publish fails", event.ID)
	}

	stats := hub.Stats()
	if stats.PublishErrors != 1 {
		t.Fatalf("publish errors = %d, want 1", stats.PublishErrors)
	}
	if stats.LastPublishError == "" {
		t.Fatal("last publish error should be recorded")
	}
}

func TestRedisInterviewEventHubReadLoopRecordsDroppedEvents(t *testing.T) {
	hub, err := NewRedisInterviewEventHub(RedisEventHubOptions{
		Addr:   "redis.test:6379",
		Buffer: 1,
		Block:  time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	server, client := net.Pipe()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan InterviewEvent, 1)
	go hub.readLoop(ctx, client, "stream", "0-0", ch)

	rawEvent := `{"type":"session.updated","session_id":"s-drop"}`
	raw := "*1\r\n" +
		"*2\r\n" +
		"$6\r\nstream\r\n" +
		"*2\r\n" +
		"*2\r\n" +
		"$3\r\n1-0\r\n" +
		"*2\r\n" +
		"$5\r\nevent\r\n" +
		"$" + strconv.Itoa(len(rawEvent)) + "\r\n" + rawEvent + "\r\n" +
		"*2\r\n" +
		"$3\r\n2-0\r\n" +
		"*2\r\n" +
		"$5\r\nevent\r\n" +
		"$" + strconv.Itoa(len(rawEvent)) + "\r\n" + rawEvent + "\r\n"
	serverErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		if _, err := server.Read(buf); err != nil {
			serverErr <- err
			return
		}
		_, err := io.WriteString(server, raw)
		serverErr <- err
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for redis response write")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hub.Stats().DroppedEvents == 1 {
			<-ch
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dropped events = %d, want 1", hub.Stats().DroppedEvents)
}

func TestRedisInterviewEventHub_IntegrationPublishSubscribe(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run Redis integration test")
	}
	rawURL := os.Getenv("INTERVIEW_REDIS_URL")
	if rawURL == "" {
		rawURL = "redis://localhost:6379/0"
	}
	opts, err := ParseRedisEventHubOptions(rawURL)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}
	opts.StreamPrefix = "interview:test:events"
	opts.Block = 100 * time.Millisecond
	opts.DialTimeout = time.Second

	hub, err := NewRedisInterviewEventHub(opts)
	if err != nil {
		t.Fatalf("new redis hub: %v", err)
	}
	defer hub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := hub.open(ctx)
	if err != nil {
		t.Fatalf("redis preflight connect: %v", err)
	}
	if got, err := redisDoString(conn, "PING"); err != nil {
		conn.Close()
		t.Fatalf("redis preflight ping: %v", err)
	} else if got != "PONG" {
		conn.Close()
		t.Fatalf("redis preflight ping = %q, want PONG", got)
	}
	conn.Close()

	sessionID := "redis-it-" + time.Now().Format("150405.000000000")
	events, unsubscribe, err := hub.Subscribe(ctx, sessionID, "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsubscribe()

	published := hub.Publish(ctx, InterviewEvent{
		Type:      interviewEventSessionUpdated,
		SessionID: sessionID,
		Status:    "running",
	})
	if published.ID == "" {
		t.Fatal("published event should have redis stream id")
	}

	select {
	case got, ok := <-events:
		if !ok {
			t.Fatal("event channel closed before receiving event")
		}
		if got.ID != published.ID {
			t.Fatalf("id = %q, want %q", got.ID, published.ID)
		}
		if got.Type != interviewEventSessionUpdated {
			t.Fatalf("type = %q, want %q", got.Type, interviewEventSessionUpdated)
		}
		if got.SessionID != sessionID {
			t.Fatalf("session_id = %q, want %q", got.SessionID, sessionID)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for redis event: %v", ctx.Err())
	}
}
