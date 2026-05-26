package httpapi

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultRedisEventStreamMaxLen = 1024

type RedisEventHubOptions struct {
	Addr         string
	Username     string
	Password     string
	DB           int
	TLS          bool
	StreamPrefix string
	Buffer       int
	Block        time.Duration
	MaxLen       int64
	DialTimeout  time.Duration
}

type RedisInterviewEventHub struct {
	opts   RedisEventHubOptions
	dialer redisDialer
	mu     sync.Mutex
	stats  RedisEventHubStats
	closed bool
}

type redisDialer func(ctx context.Context) (net.Conn, error)

type RedisEventHubStats struct {
	PublishErrors    uint64
	DroppedEvents    uint64
	LastPublishError string
}

func NewRedisInterviewEventHub(opts RedisEventHubOptions) (*RedisInterviewEventHub, error) {
	opts = opts.withDefaults()
	if opts.Addr == "" {
		return nil, fmt.Errorf("redis event hub: addr required")
	}
	h := &RedisInterviewEventHub{opts: opts}
	h.dialer = h.dial
	return h, nil
}

func ParseRedisEventHubOptions(rawURL string) (RedisEventHubOptions, error) {
	if rawURL == "" {
		return RedisEventHubOptions{}, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return RedisEventHubOptions{}, fmt.Errorf("parse redis url: %w", err)
	}
	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return RedisEventHubOptions{}, fmt.Errorf("unsupported redis scheme %q", u.Scheme)
	}
	opts := RedisEventHubOptions{
		Addr: u.Host,
		TLS:  u.Scheme == "rediss",
	}
	opts.Username = u.User.Username()
	if pw, ok := u.User.Password(); ok {
		opts.Password = pw
	}
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		db, err := strconv.Atoi(path)
		if err != nil {
			return RedisEventHubOptions{}, fmt.Errorf("parse redis db: %w", err)
		}
		opts.DB = db
	}
	return opts, nil
}

func (o RedisEventHubOptions) withDefaults() RedisEventHubOptions {
	if o.StreamPrefix == "" {
		o.StreamPrefix = "interview:events"
	}
	if o.Buffer <= 0 {
		o.Buffer = 64
	}
	if o.Block <= 0 {
		o.Block = 5 * time.Second
	}
	if o.MaxLen <= 0 {
		o.MaxLen = defaultRedisEventStreamMaxLen
	}
	if o.DialTimeout <= 0 {
		o.DialTimeout = 5 * time.Second
	}
	return o
}

func (h *RedisInterviewEventHub) Publish(ctx context.Context, event InterviewEvent) InterviewEvent {
	if h == nil || event.SessionID == "" {
		return event
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	closed := h.closed
	h.mu.Unlock()
	if closed || ctx.Err() != nil {
		return event
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return event
	}
	conn, err := h.open(ctx)
	if err != nil {
		h.recordPublishError(err)
		return event
	}
	defer conn.Close()
	id, err := redisDoString(conn,
		"XADD", h.stream(event.SessionID),
		"MAXLEN", "~", strconv.FormatInt(h.opts.MaxLen, 10),
		"*", "event", string(raw),
	)
	if err == nil {
		event.ID = id
	} else {
		h.recordPublishError(err)
	}
	return event
}

func (h *RedisInterviewEventHub) Subscribe(ctx context.Context, sessionID string, afterID string) (<-chan InterviewEvent, func(), error) {
	buffer := 64
	if h != nil && h.opts.Buffer > 0 {
		buffer = h.opts.Buffer
	}
	ch := make(chan InterviewEvent, buffer)
	if h == nil || sessionID == "" {
		return ch, func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return ch, func() {}, fmt.Errorf("event hub closed")
	}
	h.mu.Unlock()

	conn, err := h.open(ctx)
	if err != nil {
		return ch, func() {}, err
	}
	stream := h.stream(sessionID)
	lastID := strings.TrimSpace(afterID)
	if lastID == "" {
		lastID, err = redisLatestStreamID(conn, stream)
		if err != nil {
			conn.Close()
			return ch, func() {}, err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	go h.readLoop(ctx, conn, stream, lastID, ch)
	return ch, cancel, nil
}

func (h *RedisInterviewEventHub) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return nil
}

func (h *RedisInterviewEventHub) Stats() RedisEventHubStats {
	if h == nil {
		return RedisEventHubStats{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stats
}

func (h *RedisInterviewEventHub) recordPublishError(err error) {
	if h == nil || err == nil {
		return
	}
	h.mu.Lock()
	h.stats.PublishErrors++
	h.stats.LastPublishError = err.Error()
	h.mu.Unlock()
}

func (h *RedisInterviewEventHub) recordDroppedEvent() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.stats.DroppedEvents++
	h.mu.Unlock()
}

func (h *RedisInterviewEventHub) readLoop(ctx context.Context, conn net.Conn, stream string, lastID string, ch chan<- InterviewEvent) {
	defer close(ch)
	defer conn.Close()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := redisDo(conn,
			"XREAD", "BLOCK", strconv.FormatInt(h.opts.Block.Milliseconds(), 10),
			"COUNT", "32", "STREAMS", stream, lastID,
		)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		events := redisStreamEvents(res)
		for _, event := range events {
			lastID = event.ID
			select {
			case <-ctx.Done():
				return
			case ch <- event:
			default:
				h.recordDroppedEvent()
			}
		}
	}
}

func (h *RedisInterviewEventHub) stream(sessionID string) string {
	return h.opts.StreamPrefix + ":" + sessionID
}

func (h *RedisInterviewEventHub) open(ctx context.Context) (net.Conn, error) {
	if h.dialer == nil {
		h.dialer = h.dial
	}
	conn, err := h.dialer(ctx)
	if err != nil {
		return nil, err
	}
	if h.opts.Password != "" {
		args := []string{"AUTH", h.opts.Password}
		if h.opts.Username != "" {
			args = []string{"AUTH", h.opts.Username, h.opts.Password}
		}
		if _, err := redisDoString(conn, args...); err != nil {
			conn.Close()
			return nil, err
		}
	}
	if h.opts.DB > 0 {
		if _, err := redisDoString(conn, "SELECT", strconv.Itoa(h.opts.DB)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (h *RedisInterviewEventHub) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: h.opts.DialTimeout}
	if h.opts.TLS {
		return tls.DialWithDialer(&d, "tcp", h.opts.Addr, &tls.Config{MinVersion: tls.VersionTLS12})
	}
	return d.DialContext(ctx, "tcp", h.opts.Addr)
}

func redisLatestStreamID(conn net.Conn, stream string) (string, error) {
	value, err := redisDo(conn, "XREVRANGE", stream, "+", "-", "COUNT", "1")
	if err != nil {
		return "", err
	}
	messages, ok := value.([]any)
	if !ok || len(messages) == 0 {
		return "0-0", nil
	}
	message, ok := messages[0].([]any)
	if !ok || len(message) == 0 {
		return "0-0", nil
	}
	id, ok := message[0].(string)
	if !ok || id == "" {
		return "0-0", nil
	}
	return id, nil
}

func redisDoString(conn net.Conn, args ...string) (string, error) {
	value, err := redisDo(conn, args...)
	if err != nil {
		return "", err
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	default:
		return "", fmt.Errorf("redis: expected string reply, got %T", value)
	}
}

func redisDo(conn net.Conn, args ...string) (any, error) {
	if _, err := conn.Write(encodeRedisCommand(args...)); err != nil {
		return nil, err
	}
	return readRedisValue(bufio.NewReader(conn))
}

func encodeRedisCommand(args ...string) []byte {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(args)))
	b.WriteString("\r\n")
	for _, arg := range args {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(arg)))
		b.WriteString("\r\n")
		b.WriteString(arg)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func readRedisValue(r *bufio.Reader) (any, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return readRedisLine(r)
	case '-':
		line, err := readRedisLine(r)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("redis: %s", line)
	case ':':
		line, err := readRedisLine(r)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(line, 10, 64)
	case '$':
		line, err := readRedisLine(r)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		line, err := readRedisLine(r)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readRedisValue(r)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("redis: unsupported reply prefix %q", prefix)
	}
}

func readRedisLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func redisStreamEvents(value any) []InterviewEvent {
	streams, ok := value.([]any)
	if !ok {
		return nil
	}
	var out []InterviewEvent
	for _, streamRaw := range streams {
		stream, ok := streamRaw.([]any)
		if !ok || len(stream) != 2 {
			continue
		}
		messages, ok := stream[1].([]any)
		if !ok {
			continue
		}
		for _, msgRaw := range messages {
			msg, ok := msgRaw.([]any)
			if !ok || len(msg) != 2 {
				continue
			}
			id, _ := msg[0].(string)
			fields, ok := msg[1].([]any)
			if !ok {
				continue
			}
			for i := 0; i+1 < len(fields); i += 2 {
				name, _ := fields[i].(string)
				if name != "event" {
					continue
				}
				raw, _ := fields[i+1].(string)
				var event InterviewEvent
				if err := json.Unmarshal([]byte(raw), &event); err != nil {
					continue
				}
				event.ID = id
				out = append(out, event)
			}
		}
	}
	return out
}
