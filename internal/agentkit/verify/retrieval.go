package verify

import "interview-agent/internal/domain"

type RetrievalTraceVerifier struct{}

func (RetrievalTraceVerifier) VerifyRetrieval(sess *domain.Session) []Failure {
	if sess == nil || sess.RetrievalTrace == nil {
		return []Failure{{Code: "retrieval_trace_missing", Message: "retrieval trace is missing", Target: "retrieval_trace"}}
	}
	if len(sess.RetrievalTrace.Final) > 0 {
		return nil
	}
	for _, stage := range sess.RetrievalTrace.Stages {
		if len(stage.Items) > 0 {
			return nil
		}
	}
	return []Failure{{Code: "retrieval_empty", Message: "retrieval trace has no candidates", Target: "retrieval_trace.stages"}}
}
