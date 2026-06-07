package questionbank

import (
	"strings"
	"testing"
)

func TestBuildListExplainQueryUsesListQueryShape(t *testing.T) {
	query, args, err := BuildListExplainQuery(Filter{
		Query:  "Redis",
		Tags:   []string{"redis"},
		Limit:  25,
		Cursor: "40",
	})
	if err != nil {
		t.Fatalf("BuildListExplainQuery: %v", err)
	}
	for _, want := range []string{
		"EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)",
		"FROM question_bank",
		"tags @> $2::text[]",
		"id ILIKE $3 OR content ILIKE $3 OR EXISTS",
		"ORDER BY skill_category, difficulty, id",
		"LIMIT $4 OFFSET $5",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q:\n%s", want, query)
		}
	}
	if len(args) != 5 {
		t.Fatalf("args = %v, want 5 args", args)
	}
	if args[0] != "active" || args[1].([]string)[0] != "redis" || args[2] != "%Redis%" || args[3] != 26 || args[4] != 40 {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildListExplainQueryRejectsInvalidCursor(t *testing.T) {
	if _, _, err := BuildListExplainQuery(Filter{Cursor: "bad"}); err == nil {
		t.Fatal("BuildListExplainQuery returned nil error for invalid cursor")
	}
}
