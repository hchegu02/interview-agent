package retriever

import "testing"

func TestRRFMergerDedupesAndPromotesMultiSourceHits(t *testing.T) {
	got := MergeRRF([][]StageResult{
		{
			{Result: Result{ID: "a"}, Stage: StageVector, Rank: 1},
			{Result: Result{ID: "b"}, Stage: StageVector, Rank: 2},
		},
		{
			{Result: Result{ID: "b"}, Stage: StageBM25, Rank: 1},
			{Result: Result{ID: "c"}, Stage: StageBM25, Rank: 2},
		},
	}, 10, 60)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ID != "b" {
		t.Fatalf("top = %q, want b because it appears in two sources", got[0].ID)
	}
}

func TestRRFDefaultsKAndTruncates(t *testing.T) {
	got := MergeRRF([][]StageResult{{
		{Result: Result{ID: "a"}, Rank: 1},
		{Result: Result{ID: "b"}, Rank: 2},
		{Result: Result{ID: "c"}, Rank: 3},
		{Result: Result{ID: "d"}, Rank: 4},
		{Result: Result{ID: "e"}, Rank: 5},
		{Result: Result{ID: "f"}, Rank: 6},
	}}, 0, 60)
	if len(got) != 5 {
		t.Fatalf("len = %d, want default k 5", len(got))
	}
}

func TestRRFUsesPositionWhenRankIsMissing(t *testing.T) {
	got := MergeRRF([][]StageResult{{
		{Result: Result{ID: "a"}, Rank: 0},
		{Result: Result{ID: "b"}, Rank: 0},
	}}, 10, 60)
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("order = %q, %q; want position fallback order a, b", got[0].ID, got[1].ID)
	}
}

func TestRRFTieBreaksByID(t *testing.T) {
	got := MergeRRF([][]StageResult{{
		{Result: Result{ID: "z"}, Rank: 1},
		{Result: Result{ID: "a"}, Rank: 1},
	}}, 10, 60)
	if got[0].ID != "a" || got[1].ID != "z" {
		t.Fatalf("order = %q, %q; want a, z", got[0].ID, got[1].ID)
	}
}

func TestRRFSkipsEmptyID(t *testing.T) {
	got := MergeRRF([][]StageResult{
		{
			{Result: Result{}, Rank: 1},
			{Result: Result{ID: "a"}, Rank: 2},
		},
		{
			{Result: Result{}, Rank: 1},
			{Result: Result{ID: "a"}, Rank: 1},
			{Result: Result{ID: "b"}, Rank: 2},
		},
	}, 10, 60)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("order = %q, %q; want a, b", got[0].ID, got[1].ID)
	}
}

func TestRRFEmptyInputReturnsEmpty(t *testing.T) {
	got := MergeRRF(nil, 10, 60)
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestRRFDefaultsRankConstant(t *testing.T) {
	withDefault := MergeRRF([][]StageResult{{{Result: Result{ID: "a"}, Rank: 1}}}, 10, 0)
	withExplicit := MergeRRF([][]StageResult{{{Result: Result{ID: "a"}, Rank: 1}}}, 10, 60)
	if len(withDefault) != 1 || len(withExplicit) != 1 {
		t.Fatalf("unexpected lengths: default=%d explicit=%d", len(withDefault), len(withExplicit))
	}
	if withDefault[0].Score != withExplicit[0].Score {
		t.Fatalf("score = %v, want %v", withDefault[0].Score, withExplicit[0].Score)
	}
}
