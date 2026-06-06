// Command internal-trial-smoke 运行离线 internal trial 就绪 smoke gate。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"interview-agent/internal/agent"
	"interview-agent/internal/agentkit/verify"
	"interview-agent/internal/domain"
)

type smokeOptions struct {
	SessionPath          string
	BusinessFeedbackPath string
	RealGitHub           bool
}

func main() {
	var opts smokeOptions
	flag.StringVar(&opts.SessionPath, "session", defaultSessionFixturePath, "session fixture JSON path")
	flag.StringVar(&opts.BusinessFeedbackPath, "business-feedback", defaultBusinessFeedbackFixturePath, "business feedback fixture JSON path")
	flag.BoolVar(&opts.RealGitHub, "real-github", false, "reserved for explicitly configured real GitHub trial wiring")
	flag.Parse()
	os.Exit(run(opts, os.Stdout, os.Stderr))
}

const (
	defaultSessionFixturePath          = "testdata/agent_verify/pass_session.json"
	defaultBusinessFeedbackFixturePath = "testdata/internal_trial/business_feedback_pass.json"
)

func run(opts smokeOptions, stdout, stderr io.Writer) int {
	failures := []string{}
	if opts.RealGitHub {
		fmt.Fprintln(stdout, "real_github: live GitHub smoke is reserved; default smoke remains offline")
	}

	sess, err := loadSessionFixture(sessionFixturePath(opts))
	if err != nil {
		fmt.Fprintf(stderr, "load session fixture: %v\n", err)
		return 1
	}
	failures = appendFailures(failures, "interview fixture", verify.ReportCompletenessVerifier{}.VerifyReport(sess))
	failures = appendFailures(failures, "interview fixture", verify.RetrievalTraceVerifier{}.VerifyRetrieval(sess))
	failures = appendFailures(failures, "interview graph", verify.GraphStructureVerifier{}.VerifyInterviewGraph())

	feedback, err := loadBusinessFeedbackFixture(businessFeedbackFixturePath(opts))
	if err != nil {
		failures = append(failures, fmt.Sprintf("load business feedback fixture: %v", err))
	} else {
		failures = appendFailures(failures, "business feedback fixture", verify.BusinessTrialFeedbackVerifier{}.Verify(feedback))
	}

	if memoryFailures := verifyTrialMemoryObservations(); len(memoryFailures) > 0 {
		failures = append(failures, fmt.Sprintf("memory observations failed: %+v", memoryFailures))
	}

	resp, err := agent.NewDefaultService().HandleMessage(context.Background(), agent.AgentMessage{
		UserID:  "trial-user",
		Message: "请润色这个项目 https://github.com/acme/demo",
	})
	if err != nil {
		failures = append(failures, fmt.Sprintf("agent project polish failed: %v", err))
	} else {
		if resp.Intent != agent.IntentSkillProjectPolish || resp.Skill != "project_polish" {
			failures = append(failures, fmt.Sprintf("agent route = %s/%s, want project_polish", resp.Intent, resp.Skill))
		}
		if len(resp.ToolTrace) != 1 || resp.ToolTrace[0].Name != "github.project_analyze" || resp.ToolTrace[0].Status != "success" {
			failures = append(failures, fmt.Sprintf("tool trace = %+v, want github.project_analyze success", resp.ToolTrace))
		}
	}

	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(stderr, failure)
		}
		return 1
	}

	fmt.Fprintln(stdout, "interview: fixture verified with report")
	fmt.Fprintln(stdout, "memory: observations verified")
	fmt.Fprintln(stdout, "business_trial: feedback evidence verified")
	fmt.Fprintln(stdout, "project_polish: agent skill verified")
	fmt.Fprintln(stdout, "tool_trace: github.project_analyze status verified")
	return 0
}

func sessionFixturePath(opts smokeOptions) string {
	if opts.SessionPath != "" {
		return opts.SessionPath
	}
	for _, candidate := range []string{
		defaultSessionFixturePath,
		filepath.Join("..", "..", defaultSessionFixturePath),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return defaultSessionFixturePath
}

func businessFeedbackFixturePath(opts smokeOptions) string {
	if opts.BusinessFeedbackPath != "" {
		return opts.BusinessFeedbackPath
	}
	for _, candidate := range []string{
		defaultBusinessFeedbackFixturePath,
		filepath.Join("..", "..", defaultBusinessFeedbackFixturePath),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return defaultBusinessFeedbackFixturePath
}

func loadSessionFixture(path string) (*domain.Session, error) {
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

func loadBusinessFeedbackFixture(path string) (verify.BusinessTrialFeedback, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return verify.BusinessTrialFeedback{}, err
	}
	var feedback verify.BusinessTrialFeedback
	if err := json.Unmarshal(raw, &feedback); err != nil {
		return verify.BusinessTrialFeedback{}, err
	}
	return feedback, nil
}

func appendFailures(dst []string, label string, failures []verify.Failure) []string {
	if len(failures) == 0 {
		return dst
	}
	return append(dst, fmt.Sprintf("%s failed: %+v", label, failures))
}

func verifyTrialMemoryObservations() []verify.Failure {
	const sessionID = "trial-smoke-session"
	observations := []verify.MemoryPersistObservation{
		{UserID: "trial-user", SessionID: sessionID, Status: "success", Attempts: 1, ElapsedMillis: 1},
		{UserID: "trial-user", SessionID: sessionID, Status: "skipped", Reason: "report_missing", ElapsedMillis: 1},
		{UserID: "trial-user", SessionID: sessionID, Status: "failed", ErrorClass: "store_write_failed", Attempts: 1, ElapsedMillis: 1},
		{UserID: "trial-user", SessionID: sessionID, Status: "conflict_retry_exhausted", ErrorClass: "cas_conflict", Attempts: 3, ElapsedMillis: 1},
	}
	return verify.MemoryPersistVerifier{SessionID: sessionID}.VerifyMemoryObservations(observations)
}
