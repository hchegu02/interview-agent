package retriever

import (
	"math"
	"testing"
)

func TestLinearFusion_Defaults(t *testing.T) {
	f := NewLinearFusion(0, 0, 0)
	if f.WeightVector != 0.60 || f.WeightTopic != 0.30 || f.WeightDifficulty != 0.10 {
		t.Errorf("defaults wrong: %+v", f)
	}
}

func TestLinearFusion_Fuse(t *testing.T) {
	f := NewLinearFusion(0.60, 0.30, 0.10)

	// 候选 A：vector 完美 + tag 完美 + 难度命中 → score 应为 1.0
	// 候选 B：vector 一般 + tag 部分 + 难度差 1
	// 候选 C：纯 vector 命中（QueryTagCount=0 时 tag_score 应为 0）
	candidates := []Candidate{
		{ID: "A", VecDist: 0.0, TagOverlap: 2, QueryTagCount: 2, Difficulty: 3, TargetDiff: 3},
		{ID: "B", VecDist: 0.5, TagOverlap: 1, QueryTagCount: 2, Difficulty: 2, TargetDiff: 3},
		{ID: "C", VecDist: 0.2, TagOverlap: 0, QueryTagCount: 0, Difficulty: 3, TargetDiff: 3},
	}

	got := f.Fuse(candidates)
	if len(got) != 3 {
		t.Fatalf("got %d results", len(got))
	}

	// A 应排第一
	if got[0].ID != "A" {
		t.Errorf("first should be A, got %s", got[0].ID)
	}
	if math.Abs(got[0].Score-1.0) > 1e-9 {
		t.Errorf("A score = %f, want 1.0", got[0].Score)
	}

	// 验证特征分都保留
	for _, r := range got {
		if r.VectorScore < 0 || r.VectorScore > 1 {
			t.Errorf("%s: vector_score out of [0,1]: %f", r.ID, r.VectorScore)
		}
		if r.TagScore < 0 || r.TagScore > 1 {
			t.Errorf("%s: tag_score out of [0,1]: %f", r.ID, r.TagScore)
		}
		if r.DifficultyScore < 0 || r.DifficultyScore > 1 {
			t.Errorf("%s: difficulty_score out of [0,1]: %f", r.ID, r.DifficultyScore)
		}
	}

	// C 是纯 vector 命中，tag_score 应该是 0
	var cResult *Result
	for i := range got {
		if got[i].ID == "C" {
			cResult = &got[i]
		}
	}
	if cResult == nil {
		t.Fatal("C missing")
	}
	if cResult.TagScore != 0 {
		t.Errorf("C tag_score should be 0 when QueryTagCount=0, got %f", cResult.TagScore)
	}
}

func TestLinearFusion_VectorScoreClamp(t *testing.T) {
	// pgvector cosine_distance 偶尔会因浮点误差出现 -0.0000001 或 1.0000001
	// 必须 clamp 到 [0,1] 否则线性加权会产生 >1 的 score
	f := NewLinearFusion(1, 0, 0) // 只看 vector
	cs := []Candidate{
		{ID: "neg", VecDist: -0.000001, QueryTagCount: 0, Difficulty: 3, TargetDiff: 3},
		{ID: "pos", VecDist: 1.000001, QueryTagCount: 0, Difficulty: 3, TargetDiff: 3},
	}
	got := f.Fuse(cs)
	for _, r := range got {
		if r.VectorScore < 0 || r.VectorScore > 1 {
			t.Errorf("%s: vector_score not clamped: %f", r.ID, r.VectorScore)
		}
		if r.Score < 0 || r.Score > 1 {
			t.Errorf("%s: score not in [0,1]: %f", r.ID, r.Score)
		}
	}
}

func TestLinearFusion_StableOrderOnTie(t *testing.T) {
	// score 完全相同时按 ID 升序，保证测试可复现
	f := NewLinearFusion(0.60, 0.30, 0.10)
	cs := []Candidate{
		{ID: "z", VecDist: 0.5, TagOverlap: 1, QueryTagCount: 1, Difficulty: 3, TargetDiff: 3},
		{ID: "a", VecDist: 0.5, TagOverlap: 1, QueryTagCount: 1, Difficulty: 3, TargetDiff: 3},
		{ID: "m", VecDist: 0.5, TagOverlap: 1, QueryTagCount: 1, Difficulty: 3, TargetDiff: 3},
	}
	got := f.Fuse(cs)
	if got[0].ID != "a" || got[1].ID != "m" || got[2].ID != "z" {
		t.Errorf("tie-break order wrong: %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestLinearFusion_DifficultyScoreScale(t *testing.T) {
	// 难度差 0 → 1, 差 1 → 0.5, 差 2 → 0, 差 >2 → 0
	f := NewLinearFusion(0, 0, 1) // 只看难度
	cs := []Candidate{
		{ID: "0", VecDist: 0.5, Difficulty: 3, TargetDiff: 3},
		{ID: "1", VecDist: 0.5, Difficulty: 2, TargetDiff: 3},
		{ID: "2", VecDist: 0.5, Difficulty: 1, TargetDiff: 3},
	}
	got := f.Fuse(cs)
	expectScore := map[string]float64{"0": 1.0, "1": 0.5, "2": 0.0}
	for _, r := range got {
		want := expectScore[r.ID]
		if math.Abs(r.DifficultyScore-want) > 1e-9 {
			t.Errorf("%s: difficulty_score=%f, want %f", r.ID, r.DifficultyScore, want)
		}
	}
}
