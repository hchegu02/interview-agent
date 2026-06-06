package main

import (
	"bytes"
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
	for _, want := range []string{"interview", "memory", "project_polish", "tool_trace"} {
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
