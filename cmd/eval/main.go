// Command eval runs lightweight offline evaluation suites for profiles,
// scoring rules, and report fixtures.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type options struct {
	SuiteDir string
	Mode     string
	OutDir   string
}

type profileFixture struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Candidate   struct {
		Level  string   `yaml:"level" json:"level"`
		Role   string   `yaml:"role" json:"role"`
		Skills []string `yaml:"skills" json:"skills"`
	} `yaml:"candidate" json:"candidate"`
	RAG struct {
		GoldenQueries string `yaml:"golden_queries" json:"golden_queries"`
	} `yaml:"rag" json:"rag"`
	Limits map[string]int `yaml:"limits" json:"limits"`
}

type scoringFixture struct {
	ID           string             `yaml:"id" json:"id"`
	Name         string             `yaml:"name" json:"name"`
	Scale        map[string]float64 `yaml:"scale" json:"scale"`
	Weights      map[string]float64 `yaml:"weights" json:"weights"`
	PassingScore float64            `yaml:"passing_score" json:"passing_score"`
	Labels       map[string]float64 `yaml:"labels" json:"labels"`
	Cases        []scoringCase      `yaml:"cases" json:"cases"`
}

type scoringCase struct {
	ID               string   `yaml:"id" json:"id"`
	Question         string   `yaml:"question" json:"question"`
	Answer           string   `yaml:"answer" json:"answer"`
	ExpectedScoreMin float64  `yaml:"expected_score_min" json:"expected_score_min"`
	ExpectedScoreMax float64  `yaml:"expected_score_max" json:"expected_score_max"`
	ActualScore      float64  `yaml:"actual_score" json:"actual_score"`
	ExpectedHits     []string `yaml:"expected_hits" json:"expected_hits"`
	ActualHits       []string `yaml:"actual_hits" json:"actual_hits"`
	ExpectedMisses   []string `yaml:"expected_misses" json:"expected_misses"`
	ActualMisses     []string `yaml:"actual_misses" json:"actual_misses"`
}

type reportFixture struct {
	ID        string `json:"id"`
	ProfileID string `json:"profile_id"`
	ScoringID string `json:"scoring_id"`
	Summary   struct {
		TotalQueries int            `json:"total_queries"`
		Categories   map[string]int `json:"categories"`
		AverageScore float64        `json:"average_score"`
		Pass         bool           `json:"pass"`
	} `json:"summary"`
	Results []struct {
		Category string   `json:"category"`
		Score    float64  `json:"score"`
		QueryIDs []string `json:"query_ids"`
	} `json:"results"`
}

type evalSummary struct {
	Mode                 string   `json:"mode"`
	Profiles             int      `json:"profiles"`
	ScoringRules         int      `json:"scoring_rules"`
	Reports              int      `json:"reports"`
	TotalQueries         int      `json:"total_queries"`
	AverageReportScore   float64  `json:"average_report_score"`
	PassRate             float64  `json:"pass_rate"`
	ScoringCases         int      `json:"scoring_cases"`
	ScoringRangeHitRate  float64  `json:"scoring_range_hit_rate"`
	ExpectedPointHitRate float64  `json:"expected_point_hit_rate"`
	ExpectedMissHitRate  float64  `json:"expected_miss_hit_rate"`
	Issues               []string `json:"issues,omitempty"`
}

func main() {
	opts := options{}
	flag.StringVar(&opts.SuiteDir, "suite", "testdata/eval", "evaluation suite directory")
	flag.StringVar(&opts.Mode, "mode", "mock", "mock | real")
	flag.StringVar(&opts.OutDir, "out", "tmp/eval/mock", "output directory")
	flag.Parse()
	os.Exit(run(opts, os.Stdout, os.Stderr))
}

func run(opts options, stdout, stderr io.Writer) int {
	if opts.Mode == "" {
		opts.Mode = "mock"
	}
	profiles, err := loadProfiles(filepath.Join(opts.SuiteDir, "profiles"))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load profiles: %v\n", err)
		return 2
	}
	scoring, err := loadScoring(filepath.Join(opts.SuiteDir, "scoring"))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load scoring: %v\n", err)
		return 2
	}
	reports, err := loadReports(filepath.Join(opts.SuiteDir, "reports"))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load reports: %v\n", err)
		return 2
	}
	goldenIDs, err := loadProfileGoldenIDs(profiles)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load golden queries: %v\n", err)
		return 2
	}
	summary := summarize(opts.Mode, profiles, scoring, reports, goldenIDs)
	if err := writeOutputs(opts.OutDir, summary); err != nil {
		fmt.Fprintf(stderr, "ERROR: write outputs: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "eval: mode=%s profiles=%d scoring=%d reports=%d pass_rate=%.3f\n",
		summary.Mode, summary.Profiles, summary.ScoringRules, summary.Reports, summary.PassRate)
	if len(summary.Issues) > 0 {
		return 1
	}
	return 0
}

func loadProfiles(dir string) ([]profileFixture, error) {
	var out []profileFixture
	err := walkExt(dir, ".yaml", func(path string) error {
		var item profileFixture
		if err := readYAML(path, &item); err != nil {
			return err
		}
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("%s: id required", path)
		}
		out = append(out, item)
		return nil
	})
	return out, err
}

func loadScoring(dir string) ([]scoringFixture, error) {
	var out []scoringFixture
	err := walkExt(dir, ".yaml", func(path string) error {
		var item scoringFixture
		if err := readYAML(path, &item); err != nil {
			return err
		}
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("%s: id required", path)
		}
		out = append(out, item)
		return nil
	})
	return out, err
}

func loadReports(dir string) ([]reportFixture, error) {
	var out []reportFixture
	err := walkExt(dir, ".json", func(path string) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var item reportFixture
		if err := json.Unmarshal(raw, &item); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("%s: id required", path)
		}
		out = append(out, item)
		return nil
	})
	return out, err
}

func walkExt(dir, ext string, fn func(string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ext {
			continue
		}
		if err := fn(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func readYAML(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func loadProfileGoldenIDs(profiles []profileFixture) (map[string]map[string]struct{}, error) {
	out := map[string]map[string]struct{}{}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.RAG.GoldenQueries) == "" {
			continue
		}
		ids, err := loadJSONLIDs(profile.RAG.GoldenQueries)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", profile.ID, err)
		}
		out[profile.ID] = ids
	}
	return out, nil
}

func loadJSONLIDs(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]struct{}{}
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var row struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if row.ID == "" {
			return nil, fmt.Errorf("line %d: id required", lineNo)
		}
		out[row.ID] = struct{}{}
	}
	return out, scanner.Err()
}

func summarize(mode string, profiles []profileFixture, scoring []scoringFixture, reports []reportFixture, goldenIDs map[string]map[string]struct{}) evalSummary {
	s := evalSummary{Mode: mode, Profiles: len(profiles), ScoringRules: len(scoring), Reports: len(reports)}
	profileIDs := map[string]struct{}{}
	for _, p := range profiles {
		profileIDs[p.ID] = struct{}{}
		if len(p.Candidate.Skills) == 0 {
			s.Issues = append(s.Issues, p.ID+": candidate.skills empty")
		}
	}
	scoringIDs := map[string]struct{}{}
	for _, rule := range scoring {
		scoringIDs[rule.ID] = struct{}{}
		if len(rule.Weights) == 0 {
			s.Issues = append(s.Issues, rule.ID+": weights empty")
		}
		accumulateScoringCases(&s, rule)
	}
	finalizeScoringMetrics(&s)
	passed := 0
	for _, r := range reports {
		if _, ok := profileIDs[r.ProfileID]; !ok {
			s.Issues = append(s.Issues, r.ID+": profile_id not found")
		}
		if _, ok := scoringIDs[r.ScoringID]; !ok {
			s.Issues = append(s.Issues, r.ID+": scoring_id not found")
		}
		if r.Summary.TotalQueries <= 0 {
			s.Issues = append(s.Issues, r.ID+": total_queries must be positive")
		}
		categoryTotal := 0
		for _, n := range r.Summary.Categories {
			categoryTotal += n
		}
		queryRefTotal := 0
		knownQueries := goldenIDs[r.ProfileID]
		for _, item := range r.Results {
			categoryTotal -= len(item.QueryIDs)
			queryRefTotal += len(item.QueryIDs)
			for _, queryID := range item.QueryIDs {
				if len(knownQueries) > 0 {
					if _, ok := knownQueries[queryID]; !ok {
						s.Issues = append(s.Issues, r.ID+": query_id not found: "+queryID)
					}
				}
			}
		}
		if r.Summary.TotalQueries != queryRefTotal {
			s.Issues = append(s.Issues, fmt.Sprintf("%s: total_queries=%d but results reference %d queries", r.ID, r.Summary.TotalQueries, queryRefTotal))
		}
		if categoryTotal != 0 {
			s.Issues = append(s.Issues, r.ID+": category counts do not match result query ids")
		}
		s.TotalQueries += r.Summary.TotalQueries
		s.AverageReportScore += r.Summary.AverageScore
		if r.Summary.Pass {
			passed++
		}
	}
	if len(reports) > 0 {
		s.AverageReportScore /= float64(len(reports))
		s.PassRate = float64(passed) / float64(len(reports))
	}
	return s
}

func accumulateScoringCases(s *evalSummary, rule scoringFixture) {
	for _, c := range rule.Cases {
		if strings.TrimSpace(c.ID) == "" {
			s.Issues = append(s.Issues, rule.ID+": scoring case id required")
			continue
		}
		s.ScoringCases++
		if c.ExpectedScoreMin > c.ExpectedScoreMax {
			s.Issues = append(s.Issues, rule.ID+"/"+c.ID+": expected score range invalid")
			continue
		}
		if c.ActualScore >= c.ExpectedScoreMin && c.ActualScore <= c.ExpectedScoreMax {
			s.ScoringRangeHitRate++
		} else {
			s.Issues = append(s.Issues, fmt.Sprintf("%s/%s: actual_score %.2f outside [%.2f, %.2f]", rule.ID, c.ID, c.ActualScore, c.ExpectedScoreMin, c.ExpectedScoreMax))
		}
		hitRate, hitMissing := setCoverage(c.ExpectedHits, c.ActualHits)
		s.ExpectedPointHitRate += hitRate
		for _, missing := range hitMissing {
			s.Issues = append(s.Issues, rule.ID+"/"+c.ID+": expected hit not found: "+missing)
		}
		missRate, missMissing := setCoverage(c.ExpectedMisses, c.ActualMisses)
		s.ExpectedMissHitRate += missRate
		for _, missing := range missMissing {
			s.Issues = append(s.Issues, rule.ID+"/"+c.ID+": expected miss not found: "+missing)
		}
	}
}

func finalizeScoringMetrics(s *evalSummary) {
	if s.ScoringCases == 0 {
		return
	}
	n := float64(s.ScoringCases)
	s.ScoringRangeHitRate /= n
	s.ExpectedPointHitRate /= n
	s.ExpectedMissHitRate /= n
}

func setCoverage(expected, actual []string) (float64, []string) {
	if len(expected) == 0 {
		return 1, nil
	}
	actualSet := map[string]struct{}{}
	for _, item := range actual {
		item = strings.TrimSpace(item)
		if item != "" {
			actualSet[item] = struct{}{}
		}
	}
	var missing []string
	for _, item := range expected {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := actualSet[item]; !ok {
			missing = append(missing, item)
		}
	}
	denom := len(expected)
	if denom == 0 {
		return 1, missing
	}
	return float64(denom-len(missing)) / float64(denom), missing
}

func writeOutputs(outDir string, s evalSummary) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "summary.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Eval Report\n\n")
	fmt.Fprintf(&b, "- mode: `%s`\n", s.Mode)
	fmt.Fprintf(&b, "- profiles: `%d`\n", s.Profiles)
	fmt.Fprintf(&b, "- scoring_rules: `%d`\n", s.ScoringRules)
	fmt.Fprintf(&b, "- reports: `%d`\n", s.Reports)
	fmt.Fprintf(&b, "- pass_rate: `%.3f`\n", s.PassRate)
	fmt.Fprintf(&b, "- average_report_score: `%.3f`\n", s.AverageReportScore)
	if len(s.Issues) > 0 {
		fmt.Fprintf(&b, "\n## Issues\n")
		for _, issue := range s.Issues {
			fmt.Fprintf(&b, "- %s\n", issue)
		}
	}
	return os.WriteFile(filepath.Join(outDir, "report.md"), []byte(b.String()), 0o644)
}
