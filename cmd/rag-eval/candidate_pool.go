package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"interview-agent/internal/config"
	"interview-agent/internal/domain"
	"interview-agent/internal/questionbank"
	"interview-agent/internal/retriever"
)

type candidatePoolRecord struct {
	QueryID       string                   `json:"query_id"`
	SessionID     string                   `json:"session_id,omitempty"`
	Query         string                   `json:"query"`
	OriginalQuery string                   `json:"original_query,omitempty"`
	Skill         string                   `json:"skill,omitempty"`
	Tags          []string                 `json:"tags,omitempty"`
	Candidates    []candidatePoolCandidate `json:"candidates"`
}

type candidatePoolCandidate struct {
	ID      string                         `json:"id"`
	Rank    int                            `json:"rank,omitempty"`
	Score   float64                        `json:"score,omitempty"`
	Sources map[string]candidatePoolSource `json:"sources"`
}

type candidatePoolSource struct {
	Rank  int     `json:"rank,omitempty"`
	Score float64 `json:"score,omitempty"`
}

func runBuildCandidatePool(ctx context.Context, opts options, stdout, stderr io.Writer) int {
	if strings.TrimSpace(opts.SessionsPath) == "" && strings.TrimSpace(opts.QueriesPath) == "" {
		fmt.Fprintln(stderr, "ERROR: -sessions or -queries is required for build-candidate-pool")
		return 2
	}
	if strings.TrimSpace(opts.OutFile) == "" {
		fmt.Fprintln(stderr, "ERROR: -out-file is required for build-candidate-pool")
		return 2
	}
	var sessions []domain.Session
	if strings.TrimSpace(opts.SessionsPath) != "" {
		var err error
		sessions, err = loadSessionFile(opts.SessionsPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: load sessions: %v\n", err)
			return 2
		}
	}
	items, err := questionbank.LoadSeedFile(opts.SeedPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load seed: %v\n", err)
		return 2
	}
	pools := buildCandidatePoolsFromSessions(sessions, items)
	if strings.TrimSpace(opts.QueriesPath) != "" {
		queries, err := loadExportedQueryFile(opts.QueriesPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: load queries: %v\n", err)
			return 2
		}
		pools = append(pools, buildCandidatePoolsFromQueries(queries, items)...)
	}
	if opts.LiveTopK > 0 {
		if err := addLiveRetrieverCandidates(ctx, opts, pools); err != nil {
			fmt.Fprintf(stderr, "ERROR: live retriever candidates: %v\n", err)
			return 2
		}
	}
	if err := writeJSONLFile(opts.OutFile, pools); err != nil {
		fmt.Fprintf(stderr, "ERROR: write candidate pool: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "rag-eval build-candidate-pool: sessions=%d pools=%d out=%s\n", len(sessions), len(pools), opts.OutFile)
	return 0
}

func buildCandidatePoolsFromSessions(sessions []domain.Session, banks ...[]questionbank.Item) []candidatePoolRecord {
	var bank []questionbank.Item
	if len(banks) > 0 {
		bank = banks[0]
	}
	out := make([]candidatePoolRecord, 0, len(sessions))
	for _, sess := range sessions {
		if sess.RetrievalTrace == nil || strings.TrimSpace(sess.RetrievalTrace.Query) == "" {
			continue
		}
		builder := candidatePoolBuilder{byID: map[string]int{}}
		for i, candidate := range sess.CandidatePool {
			builder.add(candidate.ID, "candidate_pool", i+1, 0)
		}
		for _, item := range sess.RetrievalTrace.Final {
			builder.add(item.ID, "final", item.Rank, item.Score)
		}
		for _, stage := range sess.RetrievalTrace.Stages {
			source := "stage:" + strings.TrimSpace(stage.Stage)
			if source == "stage:" {
				source = "stage:unknown"
			}
			for _, item := range stage.Items {
				builder.add(item.ID, source, item.Rank, item.Score)
			}
		}
		addKeywordCandidates(&builder, sess.RetrievalTrace.Query, bank)
		addRandomNegativeCandidates(&builder, sess.RetrievalTrace.Query, bank, 3)
		out = append(out, candidatePoolRecord{
			QueryID:       queryID(sess),
			SessionID:     sess.ID,
			Query:         sanitizeQueryText(sess.RetrievalTrace.Query),
			OriginalQuery: sanitizeQueryText(sess.RetrievalTrace.OriginalQuery),
			Skill:         firstCandidateSkill(sess.CandidatePool),
			Tags:          firstCandidateTags(sess.CandidatePool),
			Candidates:    builder.items,
		})
	}
	return out
}

func buildCandidatePoolsFromQueries(queries []exportedQuery, bank []questionbank.Item) []candidatePoolRecord {
	out := make([]candidatePoolRecord, 0, len(queries))
	for _, query := range queries {
		if strings.TrimSpace(query.Query) == "" {
			continue
		}
		builder := candidatePoolBuilder{byID: map[string]int{}}
		for i, id := range query.CandidateIDs {
			builder.add(id, "exported_query", i+1, 0)
		}
		addKeywordCandidates(&builder, query.Query, bank)
		addRandomNegativeCandidates(&builder, query.Query, bank, 3)
		out = append(out, candidatePoolRecord{
			QueryID:       strings.TrimSpace(query.ID),
			SessionID:     strings.TrimSpace(query.SessionID),
			Query:         sanitizeQueryText(query.Query),
			OriginalQuery: sanitizeQueryText(query.OriginalQuery),
			Skill:         strings.TrimSpace(query.Skill),
			Tags:          append([]string(nil), query.Tags...),
			Candidates:    builder.items,
		})
	}
	return out
}

func addLiveRetrieverCandidates(ctx context.Context, opts options, pools []candidatePoolRecord) error {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		return fmt.Errorf("build embedder: %w", err)
	}
	r, cleanup, _, err := buildRetriever(ctx, cfg, opts.SeedPath, embedder)
	if err != nil {
		return fmt.Errorf("build retriever: %w", err)
	}
	defer cleanup()
	for i := range pools {
		queryText := strings.TrimSpace(pools[i].Query)
		if queryText == "" {
			continue
		}
		vectors, err := embedder.Embed(ctx, []string{queryText})
		if err != nil {
			return fmt.Errorf("embed query %q: %w", pools[i].QueryID, err)
		}
		if len(vectors) != 1 {
			return fmt.Errorf("embed query %q: got %d vectors, want 1", pools[i].QueryID, len(vectors))
		}
		results, err := r.Retrieve(ctx, retriever.Query{
			Text:           queryText,
			QueryEmbedding: vectors[0],
			Tags:           pools[i].Tags,
			K:              opts.LiveTopK,
		})
		if err != nil {
			return fmt.Errorf("retrieve query %q: %w", pools[i].QueryID, err)
		}
		builder := candidatePoolBuilderFromCandidates(pools[i].Candidates)
		for rank, result := range results {
			builder.add(result.ID, "live", rank+1, result.Score)
		}
		pools[i].Candidates = builder.items
	}
	return nil
}

func candidatePoolBuilderFromCandidates(candidates []candidatePoolCandidate) candidatePoolBuilder {
	builder := candidatePoolBuilder{byID: map[string]int{}}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.ID)
		if id == "" {
			continue
		}
		if candidate.Sources == nil {
			candidate.Sources = map[string]candidatePoolSource{}
		}
		builder.byID[id] = len(builder.items)
		builder.items = append(builder.items, candidate)
	}
	return builder
}

func addKeywordCandidates(builder *candidatePoolBuilder, query string, bank []questionbank.Item) {
	tokens := queryTokens(query)
	if len(tokens) == 0 {
		return
	}
	rank := 0
	for _, item := range sortedBankItems(bank) {
		if !bankItemMatchesAnyToken(item, tokens) {
			continue
		}
		rank++
		builder.add(item.ID, "keyword", rank, 0)
	}
}

func addRandomNegativeCandidates(builder *candidatePoolBuilder, query string, bank []questionbank.Item, limit int) {
	if limit <= 0 {
		return
	}
	tokens := queryTokens(query)
	rank := 0
	for _, item := range sortedBankItems(bank) {
		if _, ok := builder.byID[item.ID]; ok {
			continue
		}
		if bankItemMatchesAnyToken(item, tokens) {
			continue
		}
		rank++
		builder.add(item.ID, "random_negative", rank, 0)
		if rank >= limit {
			return
		}
	}
}

func sortedBankItems(bank []questionbank.Item) []questionbank.Item {
	out := append([]questionbank.Item(nil), bank...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func bankItemMatchesAnyToken(item questionbank.Item, tokens []string) bool {
	haystack := strings.ToLower(item.ID + " " + item.Content + " " + item.SkillCategory + " " + strings.Join(item.Tags, " "))
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

func queryTokens(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,;:!?()[]{}\"'，。；：！？（）【】")
		if len([]rune(field)) >= 2 {
			out = append(out, field)
		}
	}
	return out
}

type candidatePoolBuilder struct {
	items []candidatePoolCandidate
	byID  map[string]int
}

func (b *candidatePoolBuilder) add(id, source string, rank int, score float64) {
	id = strings.TrimSpace(id)
	source = strings.TrimSpace(source)
	if id == "" || source == "" {
		return
	}
	idx, ok := b.byID[id]
	if !ok {
		b.items = append(b.items, candidatePoolCandidate{
			ID:      id,
			Rank:    rank,
			Score:   score,
			Sources: map[string]candidatePoolSource{},
		})
		idx = len(b.items) - 1
		b.byID[id] = idx
	} else {
		if rank > 0 && (b.items[idx].Rank == 0 || rank < b.items[idx].Rank) {
			b.items[idx].Rank = rank
		}
		if score > b.items[idx].Score {
			b.items[idx].Score = score
		}
	}
	b.items[idx].Sources[source] = candidatePoolSource{Rank: rank, Score: score}
}
