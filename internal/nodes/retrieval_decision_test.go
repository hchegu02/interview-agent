package nodes

import (
	"strings"
	"testing"

	"interview-agent/internal/domain"
)

func TestRuntimeRetrievalDecisionExcludesUsedAndDetectsWeakLowInfo(t *testing.T) {
	decision := decideRuntimeRetrieval(RetrievalDecisionInput{
		Answer: "不知道",
		CandidatePool: []domain.Question{
			{ID: "q-used", Content: "已问"},
			{ID: "q-next", Content: "下一题"},
		},
		UsedQuestionIDs: []string{"q-used"},
		RetrievalTrace: &domain.RetrievalTrace{
			Final: []domain.RetrievalResultTrace{{ID: "q-next", Rank: 1, Score: 0.1}},
		},
	})

	if decision.Strategy != retrievalStrategyClarifyLowInformation {
		t.Fatalf("strategy = %s, want clarify", decision.Strategy)
	}
	if decision.IncludeContext {
		t.Fatal("weak recall decision should not include retrieval context")
	}
	if len(decision.Selected) != 1 || decision.Selected[0].ID != "q-next" {
		t.Fatalf("selected = %+v", decision.Selected)
	}
	if len(decision.ConsumedIDs) != 1 || decision.ConsumedIDs[0] != "q-used" {
		t.Fatalf("consumed = %+v", decision.ConsumedIDs)
	}
	if !decision.LowInformation || !decision.WeakRecall || decision.DegradedReason == "" {
		t.Fatalf("decision flags = %+v", decision)
	}
}

func TestRuntimeRetrievalDecisionEndsWhenAllCandidatesUsed(t *testing.T) {
	decision := decideRuntimeRetrieval(RetrievalDecisionInput{
		CandidatePool:   []domain.Question{{ID: "q1"}},
		UsedQuestionIDs: []string{"q1"},
		RetrievalTrace: &domain.RetrievalTrace{
			Final: []domain.RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.9}},
		},
	})

	if decision.Strategy != retrievalStrategyEnd {
		t.Fatalf("strategy = %s, want end", decision.Strategy)
	}
	if !strings.Contains(decision.Reason, "已排除") {
		t.Fatalf("reason should mention exclusion, got %q", decision.Reason)
	}
}

func TestRuntimeRetrievalDecisionLowInformationWithStrongRecallKeepsContext(t *testing.T) {
	decision := decideRuntimeRetrieval(RetrievalDecisionInput{
		Answer:        "不会",
		CandidatePool: []domain.Question{{ID: "q1"}},
		RetrievalTrace: &domain.RetrievalTrace{
			Stages: []domain.RetrievalStageTrace{{Stage: "rrf", Count: 2}},
			Final:  []domain.RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.8}},
		},
	})

	if decision.Strategy != retrievalStrategyClarifyLowInformation {
		t.Fatalf("strategy = %s, want clarify", decision.Strategy)
	}
	if !decision.IncludeContext {
		t.Fatal("strong recall should allow context")
	}
	if decision.DegradedReason != "" {
		t.Fatalf("degraded reason = %q, want empty", decision.DegradedReason)
	}
}

func TestRuntimeRetrievalDecisionNormalAnswerWithStrongRecallAsksNew(t *testing.T) {
	decision := decideRuntimeRetrieval(RetrievalDecisionInput{
		Answer:        "G 是 goroutine，M 是线程，P 保存本地队列并参与 work stealing。",
		CandidatePool: []domain.Question{{ID: "q1"}},
		RetrievalTrace: &domain.RetrievalTrace{
			Stages: []domain.RetrievalStageTrace{{Stage: "rrf", Count: 3}},
			Final:  []domain.RetrievalResultTrace{{ID: "q1", Rank: 1, Score: 0.9}},
		},
	})

	if decision.Strategy != retrievalStrategyAskNew || !decision.IncludeContext {
		t.Fatalf("decision = %+v, want ask_new with context", decision)
	}
	if decision.LowInformation || decision.WeakRecall {
		t.Fatalf("flags = %+v", decision)
	}
}
