// Command questionbank-lint checks whether the seed question bank is usable
// for retrieval, scoring, and interview drill-plan generation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"interview-agent/internal/questionbank"
)

type lintOptions struct {
	SeedPath          string
	MinExpectedPoints int
	MinScenarioRatio  float64
}

type lintSummary struct {
	Total                   int            `json:"total"`
	BySkill                 map[string]int `json:"by_skill"`
	ByScenario              map[string]int `json:"by_scenario"`
	ByDifficulty            map[int]int    `json:"by_difficulty"`
	MissingSkillCategory    int            `json:"missing_skill_category"`
	MissingScenario         int            `json:"missing_scenario"`
	MissingExpectedPoints   int            `json:"missing_expected_points"`
	MissingRubric           int            `json:"missing_rubric"`
	MissingSampleAnswer     int            `json:"missing_sample_answer"`
	MissingFollowUpHints    int            `json:"missing_follow_up_hints"`
	NonCanonicalTagItems    int            `json:"non_canonical_tag_items"`
	DuplicateContentItems   int            `json:"duplicate_content_items"`
	ScenarioRatio           float64        `json:"scenario_ratio"`
	ExpectedPointsPassRatio float64        `json:"expected_points_pass_ratio"`
	CompleteMetadataRatio   float64        `json:"complete_metadata_ratio"`
	Issues                  []string       `json:"issues,omitempty"`
}

func main() {
	opts := lintOptions{}
	flag.StringVar(&opts.SeedPath, "seed", "seeds/question_bank.json", "seed question bank JSON path")
	flag.IntVar(&opts.MinExpectedPoints, "min-expected-points", 3, "minimum expected_points count per item")
	flag.Float64Var(&opts.MinScenarioRatio, "min-scenario-ratio", 0.8, "minimum ratio of items with scenario")
	flag.Parse()

	code := run(opts, os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(opts lintOptions, stdout, stderr io.Writer) int {
	items, err := questionbank.LoadSeedFile(opts.SeedPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load seed: %v\n", err)
		return 2
	}
	summary := lintItems(items, opts)
	writeJSON(stdout, summary)
	if len(summary.Issues) > 0 {
		return 1
	}
	return 0
}

func lintItems(items []questionbank.Item, opts lintOptions) lintSummary {
	if opts.MinExpectedPoints <= 0 {
		opts.MinExpectedPoints = 3
	}
	if opts.MinScenarioRatio < 0 {
		opts.MinScenarioRatio = 0
	}
	if opts.MinScenarioRatio > 1 {
		opts.MinScenarioRatio = 1
	}
	s := lintSummary{
		Total:        len(items),
		BySkill:      map[string]int{},
		ByScenario:   map[string]int{},
		ByDifficulty: map[int]int{},
	}
	contentOwner := map[string]string{}
	for _, item := range items {
		s.BySkill[item.SkillCategory]++
		s.ByScenario[item.Scenario]++
		s.ByDifficulty[item.Difficulty]++
		if strings.TrimSpace(item.SkillCategory) == "" {
			s.MissingSkillCategory++
			s.Issues = append(s.Issues, fmt.Sprintf("%s: missing skill_category", item.ID))
		}
		if strings.TrimSpace(item.Scenario) == "" {
			s.MissingScenario++
		}
		if len(compact(item.ExpectedPoints)) < opts.MinExpectedPoints {
			s.MissingExpectedPoints++
			s.Issues = append(s.Issues, fmt.Sprintf("%s: expected_points below %d", item.ID, opts.MinExpectedPoints))
		}
		if len(item.Rubric) == 0 {
			s.MissingRubric++
		}
		if strings.TrimSpace(item.SampleAnswer) == "" {
			s.MissingSampleAnswer++
		}
		if len(compact(item.FollowUpHints)) == 0 {
			s.MissingFollowUpHints++
		}
		if !tagsCanonical(item.Tags) || !tagsCanonical(item.RoleTags) {
			s.NonCanonicalTagItems++
			s.Issues = append(s.Issues, fmt.Sprintf("%s: tags are not canonical", item.ID))
		}
		contentKey := normalizeContent(item.Content)
		if contentKey != "" {
			if prev, ok := contentOwner[contentKey]; ok {
				s.DuplicateContentItems++
				s.Issues = append(s.Issues, fmt.Sprintf("%s: duplicate content with %s", item.ID, prev))
			} else {
				contentOwner[contentKey] = item.ID
			}
		}
	}
	if s.Total > 0 {
		s.ScenarioRatio = float64(s.Total-s.MissingScenario) / float64(s.Total)
		s.ExpectedPointsPassRatio = float64(s.Total-s.MissingExpectedPoints) / float64(s.Total)
		complete := 0
		for _, item := range items {
			if strings.TrimSpace(item.SkillCategory) != "" &&
				strings.TrimSpace(item.Scenario) != "" &&
				len(compact(item.ExpectedPoints)) >= opts.MinExpectedPoints &&
				len(item.Rubric) > 0 &&
				strings.TrimSpace(item.SampleAnswer) != "" &&
				len(compact(item.FollowUpHints)) > 0 {
				complete++
			}
		}
		s.CompleteMetadataRatio = float64(complete) / float64(s.Total)
	}
	if s.ScenarioRatio < opts.MinScenarioRatio {
		s.Issues = append(s.Issues, fmt.Sprintf("scenario ratio %.2f below %.2f", s.ScenarioRatio, opts.MinScenarioRatio))
	}
	sort.Strings(s.Issues)
	return s
}

func tagsCanonical(tags []string) bool {
	seen := map[string]struct{}{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || trimmed != strings.ToLower(trimmed) {
			return false
		}
		if _, dup := seen[trimmed]; dup {
			return false
		}
		seen[trimmed] = struct{}{}
	}
	return true
}

func compact(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeContent(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
