package retriever

import "testing"

func TestRetrievalStageTypesPreserveSourceEvidence(t *testing.T) {
	item := StageResult{
		Result:     Result{ID: "go-gmp-001", Content: "讲一下 Go GMP 调度模型", Score: 0.75},
		Stage:      "bm25",
		Rank:       2,
		StageScore: 4.5,
		Reason:     "matched term gmp",
	}
	if item.ID != "go-gmp-001" {
		t.Fatalf("id = %q, want go-gmp-001", item.ID)
	}
	if item.Result.Score != 0.75 || item.StageScore != 4.5 {
		t.Fatalf("score evidence not preserved: %+v", item)
	}
	if item.Stage != "bm25" || item.Rank != 2 || item.Reason == "" {
		t.Fatalf("stage evidence not preserved: %+v", item)
	}
}
