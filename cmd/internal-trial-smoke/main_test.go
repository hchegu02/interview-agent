package main

import (
	"bytes"
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPassesOfflineInternalTrialSmoke(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(smokeOptions{}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"interview", "memory", "business_trial: feedback evidence verified", "project_polish", "tool_trace"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, stdout.String())
		}
	}
	if !strings.Contains(stdout.String(), "interview: fixture verified with report") {
		t.Fatalf("stdout missing verified interview marker: %s", stdout.String())
	}
}

func TestRunFailsWhenSessionFixtureIsMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(smokeOptions{SessionPath: filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run exit = %d, want non-zero; stdout = %s", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "load session fixture") {
		t.Fatalf("stderr missing load session reason: %s", stderr.String())
	}
}

func TestRunFailsWhenBusinessFeedbackFixtureIsMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(smokeOptions{BusinessFeedbackPath: filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run exit = %d, want non-zero; stdout = %s", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "load business feedback fixture") {
		t.Fatalf("stderr missing load business feedback reason: %s", stderr.String())
	}
}

func TestBusinessFeedbackFlagDefaultUsesFallback(t *testing.T) {
	var opts smokeOptions
	fs := flag.NewFlagSet("internal-trial-smoke", flag.ContinueOnError)
	registerFlags(fs, &opts)

	if opts.BusinessFeedbackPath != "" {
		t.Fatalf("BusinessFeedbackPath default = %q, want empty fallback", opts.BusinessFeedbackPath)
	}
	flag := fs.Lookup("business-feedback")
	if flag == nil {
		t.Fatal("missing business-feedback flag")
	}
	if flag.DefValue != "" {
		t.Fatalf("business-feedback flag default = %q, want empty fallback", flag.DefValue)
	}
}
