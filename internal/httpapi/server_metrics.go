package httpapi

import (
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"
)

type EventHubMetrics struct {
	PublishErrors    uint64
	DroppedEvents    uint64
	LastPublishError string
}

func (s *Server) SetInterviewService(interview *InterviewService) {
	s.interview = interview
}

func (s *Server) SetQuestionBankStore(store questionbank.Store) {
	s.questionBank = store
}

func (s *Server) GraphMetricsCallback() graph.Callback {
	return NewMetricsGraphCallback(s.metricsRecorder)
}

func (s *Server) ObserveLLMCall(record llm.CallRecord) {
	s.metricsRecorder.recordLLMCall(
		record.Model,
		record.ErrClass,
		record.Duration,
		record.PromptTokens,
		record.CompletionTokens,
	)
}

func (s *Server) WrapRetriever(r retriever.Retriever, source string) retriever.Retriever {
	return NewMetricsRetriever(r, s.metricsRecorder, source)
}

func (s *Server) SetEventHubMetrics(fn func() EventHubMetrics) {
	s.eventHubMetrics = fn
}
