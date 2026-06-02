// Command rag-eval evaluates question retrieval against golden query cases.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"

	"interview-agent/internal/config"
	"interview-agent/internal/embedding"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"
)

type evalCase struct {
	ID          string   `json:"id"`
	Query       string   `json:"query"`
	Tags        []string `json:"tags"`
	Difficulty  int      `json:"difficulty"`
	RelevantIDs []string `json:"relevant_ids"`
}

type caseResult struct {
	ID          string   `json:"id"`
	Tags        []string `json:"tags,omitempty"`
	Skill       string   `json:"skill,omitempty"`
	ReturnedIDs []string `json:"returned_ids"`
	RelevantIDs []string `json:"relevant_ids"`
	HitAt5      bool     `json:"hit_at_5"`
	HitAt10     bool     `json:"hit_at_10"`
	RecallAt5   float64  `json:"recall_at_5"`
	RecallAt10  float64  `json:"recall_at_10"`
	MRRAtK      float64  `json:"mrr_at_k"`
	NDCGAtK     float64  `json:"ndcg_at_k"`
	LatencyMS   float64  `json:"latency_ms"`
	Empty       bool     `json:"empty"`
	Fallback    bool     `json:"fallback"`
	Error       string   `json:"error,omitempty"`
}

type summary struct {
	CaseCount    int                    `json:"case_count"`
	K            int                    `json:"k"`
	Source       string                 `json:"source"`
	RecallAt5    float64                `json:"recall_at_5"`
	RecallAt10   float64                `json:"recall_at_10"`
	MRRAtK       float64                `json:"mrr_at_k"`
	NDCGAtK      float64                `json:"ndcg_at_k"`
	EmptyRate    float64                `json:"empty_rate"`
	FallbackRate float64                `json:"fallback_rate"`
	AvgLatencyMS float64                `json:"avg_latency_ms"`
	P95LatencyMS float64                `json:"p95_latency_ms"`
	ErrorCount   int                    `json:"error_count"`
	GateFailures []string               `json:"gate_failures,omitempty"`
	BySkill      map[string]groupMetric `json:"by_skill,omitempty"`
	ByTag        map[string]groupMetric `json:"by_tag,omitempty"`
	Groups       map[string]groupMetric `json:"groups,omitempty"`
	WorstGroups  []groupFailure         `json:"worst_groups,omitempty"`
	Cases        []caseResult           `json:"cases"`

	groupGatesEvaluated bool
}

type groupMetric struct {
	CaseCount  int     `json:"case_count"`
	RecallAt5  float64 `json:"recall_at_5"`
	RecallAt10 float64 `json:"recall_at_10"`
	MRRAtK     float64 `json:"mrr_at_k"`
	NDCGAtK    float64 `json:"ndcg_at_k"`
}

type groupFailure struct {
	Group     string  `json:"group"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Cases     int     `json:"cases"`
}

type groupGateOptions struct {
	MinCases      int
	MinRecallAt5  float64
	MinRecallAt10 float64
	MinMRRAtK     float64
	MinNDCGAtK    float64
	Limit         int
}

type options struct {
	CasesPath  string
	ConfigPath string
	K          int
	OutDir     string
	SeedPath   string

	MinRecallAt5  float64
	MinRecallAt10 float64
	MinMRRAtK     float64
	MinNDCGAtK    float64

	MinGroupCases      int
	MinGroupRecallAt5  float64
	MinGroupRecallAt10 float64
	MinGroupMRRAtK     float64
	MinGroupNDCGAtK    float64
}

func main() {
	opts := options{}
	flag.StringVar(&opts.CasesPath, "cases", "testdata/rag/golden_queries.jsonl", "golden query JSONL path")
	flag.StringVar(&opts.ConfigPath, "config", "config/config.yaml.example", "config YAML path")
	flag.IntVar(&opts.K, "k", 10, "top-K cutoff")
	flag.StringVar(&opts.OutDir, "out", "tmp/eval/rag", "output directory")
	flag.StringVar(&opts.SeedPath, "seed", "seeds/question_bank.json", "seed path used when PG is not configured")
	flag.Float64Var(&opts.MinRecallAt5, "min-recall-at-5", 0, "fail when recall@5 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinRecallAt10, "min-recall-at-10", 0, "fail when recall@10 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinMRRAtK, "min-mrr-at-k", 0, "fail when MRR@K is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinNDCGAtK, "min-ndcg-at-k", 0, "fail when nDCG@K is below this threshold; 0 disables")
	flag.IntVar(&opts.MinGroupCases, "min-group-cases", 0, "minimum cases required before group gates apply; 0 disables group gates")
	flag.Float64Var(&opts.MinGroupRecallAt5, "min-group-recall-at-5", 0, "fail when any eligible group recall@5 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupRecallAt10, "min-group-recall-at-10", 0, "fail when any eligible group recall@10 is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupMRRAtK, "min-group-mrr-at-k", 0, "fail when any eligible group MRR@K is below this threshold; 0 disables")
	flag.Float64Var(&opts.MinGroupNDCGAtK, "min-group-ndcg-at-k", 0, "fail when any eligible group nDCG@K is below this threshold; 0 disables")
	flag.Parse()

	code := run(context.Background(), opts, os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(ctx context.Context, opts options, stdout, stderr io.Writer) int {
	if opts.K <= 0 {
		opts.K = 10
	}
	// RAG 评估的流程故意保持线性：读 golden cases -> 构建检索器 -> 逐条评估 -> 写报告 -> 判断门槛。
	// 这样 Makefile/CI 失败时，可以直接从输出定位是数据、检索还是质量门槛的问题。
	cases, err := loadCases(opts.CasesPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load cases: %v\n", err)
		return 2
	}
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load config: %v\n", err)
		return 2
	}
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: build embedder: %v\n", err)
		return 2
	}
	r, cleanup, source, err := buildRetriever(ctx, cfg, opts.SeedPath, embedder)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: build retriever: %v\n", err)
		return 2
	}
	defer cleanup()

	result := evaluate(ctx, cases, opts.K, source, embedder, r)
	// group gate 用来防止总体指标掩盖局部退化。例如总体 recall 合格，
	// 但 redis 或 system-design 这一类问题全部检索失败，也应该让 CI 失败。
	result.WorstGroups = worstGroups(result.Groups, groupGateOptions{
		MinCases:      opts.MinGroupCases,
		MinRecallAt5:  opts.MinGroupRecallAt5,
		MinRecallAt10: opts.MinGroupRecallAt10,
		MinMRRAtK:     opts.MinGroupMRRAtK,
		MinNDCGAtK:    opts.MinGroupNDCGAtK,
	})
	result.groupGatesEvaluated = true
	result.GateFailures = thresholdFailures(result, opts)
	if err := writeOutputs(opts.OutDir, result); err != nil {
		fmt.Fprintf(stderr, "ERROR: write outputs: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "rag-eval: cases=%d recall@5=%.3f recall@10=%.3f mrr@%d=%.3f ndcg@%d=%.3f source=%s\n",
		result.CaseCount, result.RecallAt5, result.RecallAt10, opts.K, result.MRRAtK, opts.K, result.NDCGAtK, result.Source)
	if result.ErrorCount > 0 {
		return 1
	}
	for _, failure := range result.GateFailures {
		fmt.Fprintf(stderr, "FAIL: %s\n", failure)
	}
	if len(result.GateFailures) > 0 {
		return 1
	}
	return 0
}

func thresholdFailures(s summary, opts options) []string {
	var failures []string
	// 全局门槛看整体质量，group 门槛看局部质量；两者都只在阈值 > 0 时启用。
	// 这让本地调试可以临时关闭某个门槛，而 CI 使用 Makefile 中的硬阈值。
	if opts.MinRecallAt5 > 0 && s.RecallAt5 < opts.MinRecallAt5 {
		failures = append(failures, fmt.Sprintf("recall@5 %.3f below threshold %.3f", s.RecallAt5, opts.MinRecallAt5))
	}
	if opts.MinRecallAt10 > 0 && s.RecallAt10 < opts.MinRecallAt10 {
		failures = append(failures, fmt.Sprintf("recall@10 %.3f below threshold %.3f", s.RecallAt10, opts.MinRecallAt10))
	}
	if opts.MinMRRAtK > 0 && s.MRRAtK < opts.MinMRRAtK {
		failures = append(failures, fmt.Sprintf("mrr@k %.3f below threshold %.3f", s.MRRAtK, opts.MinMRRAtK))
	}
	if opts.MinNDCGAtK > 0 && s.NDCGAtK < opts.MinNDCGAtK {
		failures = append(failures, fmt.Sprintf("ndcg@k %.3f below threshold %.3f", s.NDCGAtK, opts.MinNDCGAtK))
	}
	groupFailures := s.WorstGroups
	if !s.groupGatesEvaluated {
		groupFailures = worstGroups(s.Groups, groupGateOptions{
			MinCases:      opts.MinGroupCases,
			MinRecallAt5:  opts.MinGroupRecallAt5,
			MinRecallAt10: opts.MinGroupRecallAt10,
			MinMRRAtK:     opts.MinGroupMRRAtK,
			MinNDCGAtK:    opts.MinGroupNDCGAtK,
		})
	}
	for _, failure := range groupFailures {
		failures = append(failures, fmt.Sprintf("group %s %s %.3f below threshold %.3f cases=%d",
			failure.Group, failure.Metric, failure.Value, failure.Threshold, failure.Cases))
	}
	return failures
}

func loadCases(path string) ([]evalCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []evalCase
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		// JSONL 允许空行和 # 注释，方便维护 golden queries 时按主题分组。
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c evalCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if c.ID == "" || c.Query == "" || len(c.RelevantIDs) == 0 {
			return nil, fmt.Errorf("line %d: id/query/relevant_ids required", lineNo)
		}
		out = append(out, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no cases")
	}
	return out, nil
}

func evaluate(ctx context.Context, cases []evalCase, k int, source string, embedder embedding.Embedder, r retriever.Retriever) summary {
	results := make([]caseResult, 0, len(cases))
	for _, c := range cases {
		start := time.Now()
		res := caseResult{
			ID:          c.ID,
			Tags:        retriever.CanonicalizeTags(c.Tags),
			Skill:       skillFromCase(c),
			RelevantIDs: append([]string(nil), c.RelevantIDs...),
		}
		vectors, err := embedder.Embed(ctx, []string{c.Query})
		if err != nil || len(vectors) != 1 {
			res.Error = fmt.Sprintf("embed: %v", err)
			res.Fallback = true
			res.LatencyMS = millis(time.Since(start))
			results = append(results, res)
			continue
		}
		retrieved, err := r.Retrieve(ctx, retriever.Query{
			Text:           c.Query,
			QueryEmbedding: vectors[0],
			Tags:           c.Tags,
			Difficulty:     c.Difficulty,
			K:              k,
		})
		res.LatencyMS = millis(time.Since(start))
		if err != nil {
			res.Error = fmt.Sprintf("retrieve: %v", err)
			res.Fallback = true
			results = append(results, res)
			continue
		}
		for _, item := range retrieved {
			res.ReturnedIDs = append(res.ReturnedIDs, item.ID)
		}
		res.Empty = len(res.ReturnedIDs) == 0
		res.HitAt5 = hitAt(res.ReturnedIDs, c.RelevantIDs, 5)
		res.HitAt10 = hitAt(res.ReturnedIDs, c.RelevantIDs, 10)
		res.RecallAt5 = recallAt(res.ReturnedIDs, c.RelevantIDs, 5)
		res.RecallAt10 = recallAt(res.ReturnedIDs, c.RelevantIDs, 10)
		res.MRRAtK = mrrAt(res.ReturnedIDs, c.RelevantIDs, k)
		res.NDCGAtK = ndcgAt(res.ReturnedIDs, c.RelevantIDs, k)
		results = append(results, res)
	}
	return aggregate(results, k, source)
}

func aggregate(results []caseResult, k int, source string) summary {
	out := summary{CaseCount: len(results), K: k, Source: source, Cases: results}
	latencies := make([]float64, 0, len(results))
	for _, r := range results {
		out.RecallAt5 += r.RecallAt5
		out.RecallAt10 += r.RecallAt10
		out.MRRAtK += r.MRRAtK
		out.NDCGAtK += r.NDCGAtK
		out.AvgLatencyMS += r.LatencyMS
		latencies = append(latencies, r.LatencyMS)
		if r.Empty {
			out.EmptyRate++
		}
		if r.Fallback {
			out.FallbackRate++
		}
		if r.Error != "" {
			out.ErrorCount++
		}
	}
	if len(results) > 0 {
		n := float64(len(results))
		out.RecallAt5 /= n
		out.RecallAt10 /= n
		out.MRRAtK /= n
		out.NDCGAtK /= n
		out.EmptyRate /= n
		out.FallbackRate /= n
		out.AvgLatencyMS /= n
		out.P95LatencyMS = percentile(latencies, 0.95)
	}
	out.BySkill = aggregateGroups(results, func(r caseResult) []string {
		if r.Skill == "" {
			return nil
		}
		return []string{r.Skill}
	})
	out.ByTag = aggregateGroups(results, func(r caseResult) []string {
		return r.Tags
	})
	out.Groups = map[string]groupMetric{}
	for skill, metric := range out.BySkill {
		out.Groups["skill:"+skill] = metric
	}
	for tag, metric := range out.ByTag {
		out.Groups["tag:"+tag] = metric
	}
	return out
}

func worstGroups(groups map[string]groupMetric, opts groupGateOptions) []groupFailure {
	if opts.MinCases <= 0 {
		return nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 5
	}
	var failures []groupFailure
	for name, g := range groups {
		if opts.MinCases > 0 && g.CaseCount < opts.MinCases {
			continue
		}
		if opts.MinRecallAt5 > 0 && g.RecallAt5 < opts.MinRecallAt5 {
			failures = append(failures, groupFailure{name, "recall_at_5", g.RecallAt5, opts.MinRecallAt5, g.CaseCount})
		}
		if opts.MinRecallAt10 > 0 && g.RecallAt10 < opts.MinRecallAt10 {
			failures = append(failures, groupFailure{name, "recall_at_10", g.RecallAt10, opts.MinRecallAt10, g.CaseCount})
		}
		if opts.MinMRRAtK > 0 && g.MRRAtK < opts.MinMRRAtK {
			failures = append(failures, groupFailure{name, "mrr_at_k", g.MRRAtK, opts.MinMRRAtK, g.CaseCount})
		}
		if opts.MinNDCGAtK > 0 && g.NDCGAtK < opts.MinNDCGAtK {
			failures = append(failures, groupFailure{name, "ndcg_at_k", g.NDCGAtK, opts.MinNDCGAtK, g.CaseCount})
		}
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Value == failures[j].Value {
			if failures[i].Group == failures[j].Group {
				return failures[i].Metric < failures[j].Metric
			}
			return failures[i].Group < failures[j].Group
		}
		return failures[i].Value < failures[j].Value
	})
	if len(failures) > opts.Limit {
		failures = failures[:opts.Limit]
	}
	return failures
}

func aggregateGroups(results []caseResult, keys func(caseResult) []string) map[string]groupMetric {
	out := map[string]groupMetric{}
	for _, r := range results {
		for _, key := range keys(r) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			g := out[key]
			g.CaseCount++
			g.RecallAt5 += r.RecallAt5
			g.RecallAt10 += r.RecallAt10
			g.MRRAtK += r.MRRAtK
			g.NDCGAtK += r.NDCGAtK
			out[key] = g
		}
	}
	for key, g := range out {
		n := float64(g.CaseCount)
		if n > 0 {
			g.RecallAt5 /= n
			g.RecallAt10 /= n
			g.MRRAtK /= n
			g.NDCGAtK /= n
		}
		out[key] = g
	}
	return out
}

func skillFromCase(c evalCase) string {
	for _, tag := range retriever.CanonicalizeTags(c.Tags) {
		switch tag {
		case "go", "redis", "pg", "mysql", "network", "kafka", "mq", "ai":
			return tag
		case "postgresql", "postgres":
			return "pg"
		case "system_design":
			return "system-design"
		}
	}
	if i := strings.Index(c.ID, "-"); i > 0 {
		switch prefix := c.ID[:i]; prefix {
		case "sd":
			return "system-design"
		case "pg":
			return "pg"
		default:
			return prefix
		}
	}
	return ""
}

func buildEmbedder(cfg *config.Config) (embedding.Embedder, error) {
	switch cfg.Embedding.Mode {
	case "", "mock":
		dim := cfg.Embedding.Dimension
		if dim <= 0 {
			dim = 1024
		}
		return embedding.NewMockEmbedder(dim), nil
	case "real":
		return embedding.NewRealEmbedder(cfg.Embedding.BaseURL, cfg.EmbeddingAPIKey, cfg.Embedding.Model, cfg.Embedding.Dimension, cfg.Embedding.Timeout), nil
	default:
		return nil, fmt.Errorf("unsupported embedding mode %q", cfg.Embedding.Mode)
	}
}

func buildRetriever(ctx context.Context, cfg *config.Config, seedPath string, embedder embedding.Embedder) (retriever.Retriever, func(), string, error) {
	if cfg.PostgresDSN != "" {
		pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, func() {}, "pgvector", err
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, func() {}, "pgvector", err
		}
		return retriever.NewPGVectorRetriever(pool, nil), pool.Close, "pgvector", nil
	}
	items, err := questionbank.LoadSeedFile(seedPath)
	if err != nil {
		return nil, func() {}, "seed", err
	}
	r, err := newSeedRetriever(ctx, items, embedder)
	return r, func() {}, "seed", err
}

type seedRetriever struct {
	items      []questionbank.Item
	vectors    [][]float32
	texts      []string
	fusion     retriever.Fusion
	vectorByID map[string][]float32
}

func newSeedRetriever(ctx context.Context, items []questionbank.Item, e embedding.Embedder) (*seedRetriever, error) {
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = seedItemText(item)
	}
	vectors, err := e.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(items) {
		return nil, fmt.Errorf("embedding count mismatch: got %d want %d", len(vectors), len(items))
	}
	return &seedRetriever{items: items, vectors: vectors, texts: texts, fusion: retriever.NewLinearFusion(0, 0, 0)}, nil
}

func (r *seedRetriever) Retrieve(_ context.Context, q retriever.Query) ([]retriever.Result, error) {
	if len(q.QueryEmbedding) == 0 {
		return nil, errors.New("query embedding required")
	}
	k := q.K
	if k <= 0 {
		k = 5
	}
	queryTags := retriever.CanonicalizeTags(q.Tags)
	candidates := make([]retriever.Candidate, 0, len(r.items))
	for i, item := range r.items {
		if item.Status != "" && item.Status != "active" {
			continue
		}
		vectorScore := 1 - cosineDistance(q.QueryEmbedding, r.vectors[i])
		if q.Text != "" {
			lexicalScore := lexicalSimilarity(q.Text, r.texts[i])
			vectorScore = 0.35*clamp01(vectorScore) + 0.65*lexicalScore
		}
		candidates = append(candidates, retriever.Candidate{
			ID:             item.ID,
			Content:        item.Content,
			Tags:           item.Tags,
			Difficulty:     item.Difficulty,
			Category:       item.SkillCategory,
			ExpectedPoints: append([]string(nil), item.ExpectedPoints...),
			VecDist:        1 - clamp01(vectorScore),
			TagOverlap:     tagOverlap(item.Tags, queryTags),
			QueryTagCount:  len(queryTags),
			TargetDiff:     q.Difficulty,
		})
	}
	results := r.fusion.Fuse(candidates)
	if len(results) > k {
		results = results[:k]
	}
	return results, nil
}

func seedItemText(item questionbank.Item) string {
	var b strings.Builder
	b.WriteString(item.Content)
	if len(item.ExpectedPoints) > 0 {
		b.WriteString("\nExpected: ")
		b.WriteString(strings.Join(item.ExpectedPoints, ", "))
	}
	if len(item.Tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(item.Tags, ", "))
	}
	if item.SkillCategory != "" {
		b.WriteString("\nCategory: ")
		b.WriteString(item.SkillCategory)
	}
	return b.String()
}

func lexicalSimilarity(query, doc string) float64 {
	queryTokens := textTokenWeights(query)
	if len(queryTokens) == 0 {
		return 0
	}
	docTokens := textTokenWeights(doc)
	var hit, total float64
	for token, weight := range queryTokens {
		total += weight
		if _, ok := docTokens[token]; ok {
			hit += weight
		}
	}
	if total == 0 {
		return 0
	}
	return clamp01(hit / total)
}

func textTokenWeights(s string) map[string]float64 {
	out := map[string]float64{}
	var ascii strings.Builder
	han := make([]rune, 0, 8)
	flushASCII := func() {
		if ascii.Len() == 0 {
			return
		}
		token := ascii.String()
		ascii.Reset()
		if len(token) < 2 {
			return
		}
		out[token] = 2
	}
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) == 1 {
			out[string(han[0])] = 0.5
			han = han[:0]
			return
		}
		for i := 0; i < len(han)-1; i++ {
			out[string(han[i:i+2])] = 1
		}
		han = han[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Han, r):
			flushASCII()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			ascii.WriteRune(r)
		default:
			flushASCII()
			flushHan()
		}
	}
	flushASCII()
	flushHan()
	return out
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func cosineDistance(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, aa, bb float64
	for i := 0; i < n; i++ {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return 1
	}
	sim := dot / (math.Sqrt(aa) * math.Sqrt(bb))
	return 1 - sim
}

func tagOverlap(tags, queryTags []string) int {
	if len(tags) == 0 || len(queryTags) == 0 {
		return 0
	}
	set := map[string]struct{}{}
	for _, tag := range retriever.CanonicalizeTags(tags) {
		set[tag] = struct{}{}
	}
	n := 0
	for _, tag := range queryTags {
		if _, ok := set[tag]; ok {
			n++
		}
	}
	return n
}

func hitAt(got, relevant []string, k int) bool {
	return recallAt(got, relevant, k) > 0
}

func recallAt(got, relevant []string, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	limit := k
	if len(got) < limit {
		limit = len(got)
	}
	rel := stringSet(relevant)
	hits := 0
	seen := map[string]struct{}{}
	for _, id := range got[:limit] {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := rel[id]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(rel))
}

func mrrAt(got, relevant []string, k int) float64 {
	rel := stringSet(relevant)
	limit := k
	if len(got) < limit {
		limit = len(got)
	}
	for i, id := range got[:limit] {
		if _, ok := rel[id]; ok {
			return 1 / float64(i+1)
		}
	}
	return 0
}

func ndcgAt(got, relevant []string, k int) float64 {
	rel := stringSet(relevant)
	limit := k
	if len(got) < limit {
		limit = len(got)
	}
	var dcg float64
	for i, id := range got[:limit] {
		if _, ok := rel[id]; ok {
			dcg += 1 / math.Log2(float64(i+2))
		}
	}
	ideal := len(rel)
	if ideal > k {
		ideal = k
	}
	var idcg float64
	for i := 0; i < ideal; i++ {
		idcg += 1 / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func stringSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	idx := int(math.Ceil(float64(len(values))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func writeOutputs(outDir string, s summary) error {
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
	fmt.Fprintf(&b, "# RAG Eval Report\n\n")
	fmt.Fprintf(&b, "- source: `%s`\n", s.Source)
	fmt.Fprintf(&b, "- cases: `%d`\n", s.CaseCount)
	fmt.Fprintf(&b, "- recall@5: `%.3f`\n", s.RecallAt5)
	fmt.Fprintf(&b, "- recall@10: `%.3f`\n", s.RecallAt10)
	fmt.Fprintf(&b, "- mrr@%d: `%.3f`\n", s.K, s.MRRAtK)
	fmt.Fprintf(&b, "- ndcg@%d: `%.3f`\n", s.K, s.NDCGAtK)
	fmt.Fprintf(&b, "- empty_rate: `%.3f`\n", s.EmptyRate)
	fmt.Fprintf(&b, "- fallback_rate: `%.3f`\n", s.FallbackRate)
	fmt.Fprintf(&b, "- p95_latency_ms: `%.3f`\n", s.P95LatencyMS)
	return os.WriteFile(filepath.Join(outDir, "report.md"), []byte(b.String()), 0o644)
}
