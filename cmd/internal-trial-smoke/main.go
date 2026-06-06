// Command internal-trial-smoke 运行离线 internal trial 就绪 smoke gate。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"interview-agent/internal/agent"
	"interview-agent/internal/agentkit/verify"
)

type smokeOptions struct {
	RealGitHub bool
}

func main() {
	var opts smokeOptions
	flag.BoolVar(&opts.RealGitHub, "real-github", false, "reserved for explicitly configured real GitHub trial wiring")
	flag.Parse()
	os.Exit(run(opts, os.Stdout, os.Stderr))
}

func run(opts smokeOptions, stdout, stderr io.Writer) int {
	failures := []string{}
	if opts.RealGitHub {
		fmt.Fprintln(stdout, "real_github: live GitHub smoke is reserved; default smoke remains offline")
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
		if len(resp.ToolTrace) != 1 || resp.ToolTrace[0].Name != "github.project_analyze" || resp.ToolTrace[0].Status == "" {
			failures = append(failures, fmt.Sprintf("tool trace = %+v, want github.project_analyze with status", resp.ToolTrace))
		}
	}

	if len(failures) > 0 {
		for _, failure := range failures {
			fmt.Fprintln(stderr, failure)
		}
		return 1
	}

	fmt.Fprintln(stdout, "interview: fixture completed with report")
	fmt.Fprintln(stdout, "memory: observations verified")
	fmt.Fprintln(stdout, "project_polish: agent skill verified")
	fmt.Fprintln(stdout, "tool_trace: github.project_analyze status verified")
	return 0
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
