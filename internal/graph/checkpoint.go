package graph

import (
	"context"
	"sync"
	"time"
)

// CheckpointPhase 表示 checkpoint 记录的是 Graph 执行的哪个阶段。
type CheckpointPhase string

const (
	CheckpointFrontierBefore CheckpointPhase = "frontier_before"
	CheckpointFrontierAfter  CheckpointPhase = "frontier_after"
	CheckpointFrontierError  CheckpointPhase = "frontier_error"
	CheckpointNodeBefore     CheckpointPhase = "node_before"
	CheckpointNodeAfter      CheckpointPhase = "node_after"
	CheckpointNodeError      CheckpointPhase = "node_error"
	CheckpointSuspended      CheckpointPhase = "suspended"
	CheckpointResumeFrom     CheckpointPhase = "resume_from"
)

// GraphCheckpoint 是 Graph 执行过程中的轻量状态快照。
type GraphCheckpoint struct {
	Seq          int64           `json:"seq"`
	SessionID    string          `json:"session_id,omitempty"`
	Step         int             `json:"step"`
	Graph        string          `json:"graph"`
	Phase        CheckpointPhase `json:"phase"`
	Frontier     []string        `json:"frontier,omitempty"`
	Node         string          `json:"node,omitempty"`
	Error        string          `json:"error,omitempty"`
	PatchSummary *PatchSummary   `json:"patch_summary,omitempty"`
	Snapshot     []byte          `json:"snapshot,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// PatchSummary 只记录 patch 的字段级摘要，不记录完整业务载荷。
type PatchSummary struct {
	Node               string   `json:"node"`
	Writes             []string `json:"writes,omitempty"`
	WrittenFields      []string `json:"written_fields,omitempty"`
	CurrentRoundID     string   `json:"current_round_id,omitempty"`
	DegradedComponents []string `json:"degraded_components,omitempty"`
	Suspended          bool     `json:"suspended,omitempty"`
	IdempotencyKey     string   `json:"idempotency_key,omitempty"`
}

// CheckpointRecorder 接收 Graph runner 产生的 checkpoint。
// 实现应快速返回并尊重 ctx；runner 会限制等待时间并隔离 panic，
// 但无法强制终止一个完全忽略 ctx 的 recorder。
type CheckpointRecorder interface {
	RecordCheckpoint(ctx context.Context, checkpoint GraphCheckpoint)
}

// MemoryCheckpointRecorder 是只保留最近 N 条 checkpoint 的内存 ring buffer。
type MemoryCheckpointRecorder struct {
	mu       sync.Mutex
	capacity int
	nextSeq  int64
	items    []GraphCheckpoint
}

// NewMemoryCheckpointRecorder 构造内存 checkpoint recorder。
func NewMemoryCheckpointRecorder(capacity int) *MemoryCheckpointRecorder {
	if capacity <= 0 {
		capacity = 100
	}
	return &MemoryCheckpointRecorder{capacity: capacity}
}

func (r *MemoryCheckpointRecorder) RecordCheckpoint(ctx context.Context, checkpoint GraphCheckpoint) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextSeq++
	checkpoint.Seq = r.nextSeq
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = time.Now().UTC()
	}
	checkpoint.Frontier = append([]string(nil), checkpoint.Frontier...)
	checkpoint.Snapshot = append([]byte(nil), checkpoint.Snapshot...)
	if checkpoint.PatchSummary != nil {
		checkpoint.PatchSummary = clonePatchSummary(checkpoint.PatchSummary)
	}

	if len(r.items) == r.capacity {
		copy(r.items, r.items[1:])
		r.items[len(r.items)-1] = checkpoint
		return
	}
	r.items = append(r.items, checkpoint)
}

// Snapshot 返回当前 ring buffer 内容的拷贝。
func (r *MemoryCheckpointRecorder) Snapshot() []GraphCheckpoint {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]GraphCheckpoint, len(r.items))
	for i, item := range r.items {
		out[i] = item
		out[i].Frontier = append([]string(nil), item.Frontier...)
		out[i].Snapshot = append([]byte(nil), item.Snapshot...)
		if item.PatchSummary != nil {
			out[i].PatchSummary = clonePatchSummary(item.PatchSummary)
		}
	}
	return out
}

func clonePatchSummary(summary *PatchSummary) *PatchSummary {
	if summary == nil {
		return nil
	}
	clone := *summary
	clone.Writes = append([]string(nil), summary.Writes...)
	clone.WrittenFields = append([]string(nil), summary.WrittenFields...)
	clone.DegradedComponents = append([]string(nil), summary.DegradedComponents...)
	return &clone
}
