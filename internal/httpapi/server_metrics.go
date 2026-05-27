package httpapi

import (
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

func (s *Server) SetInterviewService(interview *InterviewService) {
	s.interview = interview
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
