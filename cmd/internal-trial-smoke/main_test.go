package main

import (
	"bytes"
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
}
