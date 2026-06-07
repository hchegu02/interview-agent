// Command questionbank-explain prints the PostgreSQL execution plan for the
// question bank list query.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/config"
	"interview-agent/internal/questionbank"
)

type options struct {
	ConfigPath    string
	Query         string
	SkillCategory string
	Scenario      string
	Difficulty    int
	TagsCSV       string
	Status        string
	Limit         int
	Cursor        string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.ConfigPath, "config", "config/config.yaml.example", "config YAML path")
	flag.StringVar(&opts.Query, "query", "", "question bank search query")
	flag.StringVar(&opts.SkillCategory, "skill", "", "skill_category filter")
	flag.StringVar(&opts.Scenario, "scenario", "", "scenario filter")
	flag.IntVar(&opts.Difficulty, "difficulty", 0, "difficulty filter")
	flag.StringVar(&opts.TagsCSV, "tags", "", "comma-separated tags filter")
	flag.StringVar(&opts.Status, "status", "", "status filter; default active")
	flag.IntVar(&opts.Limit, "limit", 20, "list page size")
	flag.StringVar(&opts.Cursor, "cursor", "", "offset cursor")
	flag.Parse()

	os.Exit(run(opts))
}

func run(opts options) int {
	ctx := context.Background()
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: load config: %v\n", err)
		return 2
	}
	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		fmt.Fprintln(os.Stderr, "ERROR: postgres_dsn is required for questionbank-explain")
		return 2
	}
	poolCfg, err := config.PostgresPoolConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: postgres pool config: %v\n", err)
		return 2
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: connect postgres: %v\n", err)
		return 2
	}
	defer pool.Close()
	store := questionbank.NewPGStore(pool)
	lines, err := store.ExplainList(ctx, buildFilter(opts))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: explain question bank list: %v\n", err)
		return 2
	}
	for _, line := range lines {
		fmt.Fprintln(os.Stdout, line)
	}
	return 0
}

func buildFilter(opts options) questionbank.Filter {
	return questionbank.Filter{
		Query:         strings.TrimSpace(opts.Query),
		SkillCategory: strings.TrimSpace(opts.SkillCategory),
		Scenario:      strings.TrimSpace(opts.Scenario),
		Difficulty:    opts.Difficulty,
		Tags:          splitCSV(opts.TagsCSV),
		Status:        strings.TrimSpace(opts.Status),
		Limit:         opts.Limit,
		Cursor:        strings.TrimSpace(opts.Cursor),
	}
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
