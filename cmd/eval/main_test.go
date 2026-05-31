package main

import "testing"

func TestSummarizeDetectsFixtureLinks(t *testing.T) {
	profiles := []profileFixture{{ID: "p1"}}
	profiles[0].Candidate.Skills = []string{"go"}
	scoring := []scoringFixture{{ID: "s1", Weights: map[string]float64{"correctness": 1}}}
	reports := []reportFixture{{ID: "r1", ProfileID: "p1", ScoringID: "missing"}}
	reports[0].Summary.TotalQueries = 10
	reports[0].Summary.AverageScore = 3.5
	reports[0].Summary.Pass = true

	got := summarize("mock", profiles, scoring, reports, nil)

	if got.Reports != 1 || got.TotalQueries != 10 || got.PassRate != 1 {
		t.Fatalf("summary = %+v", got)
	}
	if len(got.Issues) != 2 || got.Issues[0] != "r1: scoring_id not found" {
		t.Fatalf("issues = %+v", got.Issues)
	}
}

func TestSummarizeScoringCases(t *testing.T) {
	scoring := []scoringFixture{{
		ID:      "s1",
		Weights: map[string]float64{"correctness": 1},
		Cases: []scoringCase{{
			ID:               "case-1",
			ExpectedScoreMin: 3,
			ExpectedScoreMax: 4,
			ActualScore:      3.5,
			ExpectedHits:     []string{"锁续期", "Lua 解锁"},
			ActualHits:       []string{"Lua 解锁", "锁续期"},
			ExpectedMisses:   []string{"Redlock 风险"},
			ActualMisses:     []string{"Redlock 风险"},
		}},
	}}

	got := summarize("mock", nil, scoring, nil, nil)

	if got.ScoringCases != 1 {
		t.Fatalf("scoring cases = %d, want 1", got.ScoringCases)
	}
	if got.ScoringRangeHitRate != 1 || got.ExpectedPointHitRate != 1 || got.ExpectedMissHitRate != 1 {
		t.Fatalf("scoring metrics = %+v", got)
	}
}
