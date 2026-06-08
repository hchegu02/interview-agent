package questionbank

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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

func TestScanItemAcceptsNullEmbeddedAt(t *testing.T) {
	now := time.Now().UTC()
	row := fakeItemScanner{values: []any{
		"scan-null-embedded-at",
		"PostgreSQL 慢查询如何定位？",
		[]string{"postgresql"},
		"postgresql",
		4,
		[]string{"EXPLAIN", "pg_stat_statements"},
		"manual",
		"",
		[]string(nil),
		[]byte(`{"pass":"能说明执行计划"}`),
		"",
		[]string(nil),
		"zh-CN",
		"active",
		"failed",
		"",
		pgtype.Timestamptz{},
		"embedding backend unavailable",
		now,
		now,
	}}

	item, err := scanItem(row)
	if err != nil {
		t.Fatalf("scanItem: %v", err)
	}
	if item.EmbeddingStatus != "failed" || !item.EmbeddedAt.IsZero() {
		t.Fatalf("item = %+v, want failed with zero EmbeddedAt", item)
	}
}

type fakeItemScanner struct {
	values []any
}

func (r fakeItemScanner) Scan(dest ...any) error {
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = r.values[i].(string)
		case *int:
			*d = r.values[i].(int)
		case *[]string:
			if v, ok := r.values[i].([]string); ok {
				*d = v
			}
		case *[]byte:
			*d = r.values[i].([]byte)
		case *time.Time:
			*d = r.values[i].(time.Time)
		case *pgtype.Timestamptz:
			*d = r.values[i].(pgtype.Timestamptz)
		}
	}
	return nil
}
