package interviewagent

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
		"real-rag-reindex:",
		"demo-real-full:",
	} {
		if !strings.Contains(makefile, marker) {
			t.Fatalf("Makefile missing target marker %q", marker)
		}
	}
}

func TestMakefileAppliesAllMigrationsInOrder(t *testing.T) {
	makefile := readTextFile(t, "Makefile")
	upBlock := makeTargetBlock(makefile, "migrate-up")
	downBlock := makeTargetBlock(makefile, "migrate-down")
	wantUp, wantDown := migrationFiles(t)
	assertOrderedMarkers(t, upBlock, wantUp)
	assertOrderedMarkers(t, downBlock, wantDown)
}

func TestRealDemoScriptsDocumentAndAssertRealChain(t *testing.T) {
	readme := readTextFile(t, "README.md")
	for _, marker := range []string{
		"真实完整演示",
		"scripts/real_e2e.ps1",
		"INTERVIEW_EMBEDDING_API_KEY",
		"INTERVIEW_EMBEDDING_BASE_URL",
		"run.json.config.retriever",
	} {
		if !strings.Contains(readme, marker) {
			t.Fatalf("README.md missing real demo marker %q", marker)
		}
	}

	migrationReadme := readTextFile(t, "migrations/README.md")
	if !strings.Contains(migrationReadme, "008_question_bank_import_field_provenance.up.sql") {
		t.Fatal("migrations/README.md should list the latest migration")
	}

	script := readTextFile(t, "scripts/real_e2e.ps1")
	for _, marker := range []string{
		"INTERVIEW_LLM_API_KEY",
		"INTERVIEW_EMBEDDING_API_KEY",
		"INTERVIEW_EMBEDDING_BASE_URL",
		"INTERVIEW_POSTGRES_DSN",
		"cmd/reindex",
		"-base-url",
		"cmd/demo",
		"retriever",
		"pgvector",
		"report",
		"llm_calls",
		"nodes",
	} {
		if !strings.Contains(script, marker) {
			t.Fatalf("scripts/real_e2e.ps1 missing marker %q", marker)
		}
	}
}

func makeTargetBlock(makefile, target string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:.*(?:\n(?:\t.*|[ \t]*#.*))*`)
	return re.FindString(makefile)
}

func assertOrderedMarkers(t *testing.T, text string, markers []string) {
	t.Helper()
	if text == "" {
		t.Fatal("target block is empty")
	}
	pos := -1
	for _, marker := range markers {
		next := strings.Index(text, marker)
		if next < 0 {
			t.Fatalf("missing marker %q in block:\n%s", marker, text)
		}
		if next <= pos {
			t.Fatalf("marker %q appears out of order in block:\n%s", marker, text)
		}
		pos = next
	}
}

func migrationFiles(t *testing.T) ([]string, []string) {
	t.Helper()
	upMatches, err := filepath.Glob(filepath.Join("migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob up migrations: %v", err)
	}
	downMatches, err := filepath.Glob(filepath.Join("migrations", "*.down.sql"))
	if err != nil {
		t.Fatalf("glob down migrations: %v", err)
	}
	if len(upMatches) == 0 {
		t.Fatal("no up migrations found")
	}
	if len(upMatches) != len(downMatches) {
		t.Fatalf("migration up/down count mismatch: up=%d down=%d", len(upMatches), len(downMatches))
	}
	sort.Strings(upMatches)
	sort.Sort(sort.Reverse(sort.StringSlice(downMatches)))
	return slashPaths(upMatches), slashPaths(downMatches)
}

func slashPaths(paths []string) []string {
	out := make([]string, len(paths))
	for i, path := range paths {
		out[i] = filepath.ToSlash(path)
	}
	return out
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
