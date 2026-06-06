package graph

import (
	"context"
	"testing"
)

func TestMemoryCheckpointRecorder_RingBufferAndCopies(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(2)

	firstFrontier := []string{"a"}
	firstSnapshot := []byte(`{"id":"s1"}`)
	rec.RecordCheckpoint(context.Background(), GraphCheckpoint{
		SessionID: "s1",
		Phase:     CheckpointFrontierBefore,
		Frontier:  firstFrontier,
		Snapshot:  firstSnapshot,
	})
	firstFrontier[0] = "mutated"
	firstSnapshot[0] = 'X'

	rec.RecordCheckpoint(context.Background(), GraphCheckpoint{SessionID: "s2", Phase: CheckpointFrontierBefore})
	rec.RecordCheckpoint(context.Background(), GraphCheckpoint{SessionID: "s3", Phase: CheckpointFrontierBefore})

	got := rec.Snapshot()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SessionID != "s2" || got[1].SessionID != "s3" {
		t.Fatalf("ring buffer should keep latest entries: %+v", got)
	}
	if got[0].Seq != 2 || got[1].Seq != 3 {
		t.Fatalf("seq should be assigned by recorder: %+v", got)
	}
}

func TestMemoryCheckpointRecorder_SnapshotReturnsCopy(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(1)
	rec.RecordCheckpoint(context.Background(), GraphCheckpoint{
		SessionID: "s1",
		Frontier:  []string{"a"},
		PatchSummary: &PatchSummary{
			Node:               "a",
			Writes:             []string{"working_memory"},
			WrittenFields:      []string{"working_memory"},
			DegradedComponents: []string{"rag"},
		},
		Snapshot: []byte(`{"id":"s1"}`),
	})

	first := rec.Snapshot()
	first[0].Frontier[0] = "mutated"
	first[0].Snapshot[0] = 'X'
	first[0].PatchSummary.Writes[0] = "mutated"
	first[0].PatchSummary.WrittenFields[0] = "mutated"
	first[0].PatchSummary.DegradedComponents[0] = "mutated"

	second := rec.Snapshot()
	if second[0].Frontier[0] != "a" {
		t.Fatalf("frontier should be copied: %+v", second[0].Frontier)
	}
	if string(second[0].Snapshot) != `{"id":"s1"}` {
		t.Fatalf("snapshot should be copied: %s", string(second[0].Snapshot))
	}
	if second[0].PatchSummary.Writes[0] != "working_memory" {
		t.Fatalf("patch summary writes should be copied: %+v", second[0].PatchSummary)
	}
	if second[0].PatchSummary.WrittenFields[0] != "working_memory" {
		t.Fatalf("patch summary written fields should be copied: %+v", second[0].PatchSummary)
	}
	if second[0].PatchSummary.DegradedComponents[0] != "rag" {
		t.Fatalf("patch summary degraded components should be copied: %+v", second[0].PatchSummary)
	}
}

func TestMemoryCheckpointRecorder_DefaultCapacity(t *testing.T) {
	rec := NewMemoryCheckpointRecorder(0)
	for i := 0; i < 101; i++ {
		rec.RecordCheckpoint(context.Background(), GraphCheckpoint{Phase: CheckpointFrontierBefore})
	}
	got := rec.Snapshot()
	if len(got) != 100 {
		t.Fatalf("default capacity len = %d, want 100", len(got))
	}
}
