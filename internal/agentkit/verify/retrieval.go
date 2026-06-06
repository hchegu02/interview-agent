package verify

import (
	"fmt"
	"strings"

	"interview-agent/internal/domain"
)

type RetrievalTraceVerifier struct{}

func (RetrievalTraceVerifier) VerifyRetrieval(sess *domain.Session) []Failure {
	if sess == nil || sess.RetrievalTrace == nil {
		return []Failure{{Code: "retrieval_trace_missing", Message: "retrieval trace is missing", Target: "retrieval_trace"}}
	}
	failures := []Failure{}
	if strings.TrimSpace(sess.RetrievalTrace.Query) == "" {
		failures = append(failures, Failure{Code: "retrieval_query_missing", Message: "retrieval query is missing", Target: "retrieval_trace.query"})
	}
	if len(sess.RetrievalTrace.Final) > 0 {
		failures = append(failures, verifyRetrievalItems("retrieval_trace.final", sess.RetrievalTrace.Final, true)...)
		if len(failures) == 0 {
			return nil
		}
		return failures
	}
	for _, stage := range sess.RetrievalTrace.Stages {
		if strings.TrimSpace(stage.Stage) == "" {
			failures = append(failures, Failure{Code: "retrieval_stage_name_missing", Message: "retrieval stage name is missing", Target: "retrieval_trace.stages"})
		}
		failures = append(failures, verifyRetrievalItems("retrieval_trace.stages.items", stage.Items, false)...)
		if len(stage.Items) > 0 {
			return failures
		}
	}
	failures = append(failures, Failure{Code: "retrieval_empty", Message: "retrieval trace has no candidates", Target: "retrieval_trace.stages"})
	return failures
}

func verifyRetrievalItems(target string, items []domain.RetrievalResultTrace, requireUniqueID bool) []Failure {
	failures := []Failure{}
	seen := map[string]bool{}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			failures = append(failures, Failure{Code: "retrieval_item_id_missing", Message: "retrieval item id is missing", Target: target})
		}
		if item.Rank < 0 {
			failures = append(failures, Failure{Code: "retrieval_rank_invalid", Message: fmt.Sprintf("retrieval item %q rank %d is invalid", item.ID, item.Rank), Target: target})
		}
		if item.Score < 0 {
			failures = append(failures, Failure{Code: "retrieval_score_invalid", Message: fmt.Sprintf("retrieval item %q score %f is invalid", item.ID, item.Score), Target: target})
		}
		if requireUniqueID && id != "" {
			if seen[id] {
				failures = append(failures, Failure{Code: "retrieval_final_duplicate_id", Message: fmt.Sprintf("retrieval final contains duplicate id %q", id), Target: target})
			}
			seen[id] = true
		}
	}
	return failures
}
