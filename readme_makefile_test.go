package interviewagent

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeDocumentsWebDemoAndRoadmap(t *testing.T) {
	readme := readTextFile(t, "README.md")
	for _, marker := range []string{
		"Web 前端",
		"make demo-web",
		"POST /api/documents/parse-resume",
		"Prometheus metrics",
		"OTel tracing",
	} {
		if !strings.Contains(readme, marker) {
			t.Fatalf("README.md missing marker %q", marker)
		}
	}
}

func TestMakefileExposesDemoAndCoreTestTargets(t *testing.T) {
	makefile := readTextFile(t, "Makefile")
	for _, marker := range []string{
		"demo-web:",
		"demo-web-real:",
		"test-core:",
	} {
		if !strings.Contains(makefile, marker) {
			t.Fatalf("Makefile missing target marker %q", marker)
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
