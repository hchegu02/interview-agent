package verify

import "strings"

type MemoryPersistObservation struct {
	UserID        string `json:"user_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
	ErrorClass    string `json:"error_class,omitempty"`
	Attempts      int    `json:"attempts,omitempty"`
	ElapsedMillis int64  `json:"elapsed_ms,omitempty"`
}

type MemoryPersistVerifier struct {
	SessionID string
}

func (v MemoryPersistVerifier) VerifyMemoryObservations(observations []MemoryPersistObservation) []Failure {
	failures := []Failure{}
	for _, observation := range observations {
		target := strings.TrimSpace(observation.SessionID)
		if target == "" {
			target = v.SessionID
		}
		if v.SessionID != "" && strings.TrimSpace(observation.SessionID) != "" && strings.TrimSpace(observation.SessionID) != v.SessionID {
			failures = append(failures, Failure{
				Code:    "memory_session_mismatch",
				Message: "memory observation session_id does not match verified session",
				Target:  target,
			})
		}
		switch strings.TrimSpace(observation.Status) {
		case "success":
			if observation.Attempts <= 0 {
				failures = append(failures, Failure{Code: "memory_attempts_missing", Message: "successful memory observation must include attempts", Target: target})
			}
		case "skipped":
			if strings.TrimSpace(observation.Reason) == "" {
				failures = append(failures, Failure{Code: "memory_skip_reason_missing", Message: "skipped memory observation must include reason", Target: target})
			}
		case "failed", "conflict_retry_exhausted":
			if strings.TrimSpace(observation.ErrorClass) == "" {
				failures = append(failures, Failure{Code: "memory_error_class_missing", Message: "failed memory observation must include error_class", Target: target})
			}
			if observation.Attempts <= 0 {
				failures = append(failures, Failure{Code: "memory_attempts_missing", Message: "failed memory observation must include attempts", Target: target})
			}
		default:
			failures = append(failures, Failure{Code: "memory_status_invalid", Message: "memory observation status is invalid", Target: target})
		}
		if observation.ElapsedMillis <= 0 {
			failures = append(failures, Failure{Code: "memory_elapsed_missing", Message: "memory observation must include positive elapsed_ms", Target: target})
		}
	}
	return failures
}
