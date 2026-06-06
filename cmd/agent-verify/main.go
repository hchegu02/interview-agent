// Command agent-verify runs lightweight Agent output verification gates.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/agentkit/verify"
	"interview-agent/internal/domain"
)

type options struct {
	SessionPath            string
	ToolEventsPath         string
	MemoryObservationsPath string
}

type verifySummary struct {
	Pass         bool             `json:"pass"`
	SessionID    string           `json:"session_id,omitempty"`
	FailureCount int              `json:"failure_count"`
	Failures     []verify.Failure `json:"failures,omitempty"`
}

func main() {
	opts := options{}
	flag.StringVar(&opts.SessionPath, "session", "", "session JSON file path")
	flag.StringVar(&opts.ToolEventsPath, "tool-events", "", "optional hook events JSON file path")
	flag.StringVar(&opts.MemoryObservationsPath, "memory-observations", "", "optional long-term memory observations JSON file path")
	flag.Parse()
	os.Exit(run(opts, os.Stdout, os.Stderr))
}

func run(opts options, stdout, stderr io.Writer) int {
	if opts.SessionPath == "" {
		fmt.Fprintln(stderr, "ERROR: -session is required")
		return 2
	}

	sess, err := loadSession(opts.SessionPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load session: %v\n", err)
		return 2
	}

	failures := []verify.Failure{}
	failures = append(failures, verify.ReportCompletenessVerifier{}.VerifyReport(sess)...)
	failures = append(failures, verify.RetrievalTraceVerifier{}.VerifyRetrieval(sess)...)
	failures = append(failures, verify.GraphStructureVerifier{}.VerifyInterviewGraph()...)

	if opts.ToolEventsPath != "" {
		events, err := loadToolEvents(opts.ToolEventsPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: load tool events: %v\n", err)
			return 2
		}
		failures = append(failures, verify.ToolCallVerifier{}.VerifyToolEvents(events)...)
	}
	if opts.MemoryObservationsPath != "" {
		observations, err := loadMemoryObservations(opts.MemoryObservationsPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: load memory observations: %v\n", err)
			return 2
		}
		failures = append(failures, verify.MemoryPersistVerifier{SessionID: sess.ID}.VerifyMemoryObservations(observations)...)
	}

	summary := verifySummary{
		Pass:         len(failures) == 0,
		SessionID:    sess.ID,
		FailureCount: len(failures),
		Failures:     failures,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		fmt.Fprintf(stderr, "ERROR: write summary: %v\n", err)
		return 2
	}
	if len(failures) > 0 {
		return 1
	}
	return 0
}

func loadSession(path string) (*domain.Session, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess domain.Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func loadToolEvents(path string) ([]agentkit.HookEvent, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []agentkit.HookEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func loadMemoryObservations(path string) ([]verify.MemoryPersistObservation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var observations []verify.MemoryPersistObservation
	if err := json.Unmarshal(raw, &observations); err != nil {
		return nil, err
	}
	return observations, nil
}
