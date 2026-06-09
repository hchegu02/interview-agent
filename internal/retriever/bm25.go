package retriever

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
)

var bm25TokenRE = regexp.MustCompile(`[A-Za-z0-9_+#.-]+|[\p{Han}]+`)

// BM25Retriever 是基于本地文档的 BM25 检索器。
type BM25Retriever struct {
	docs   []Result
	tf     []map[string]int
	df     map[string]int
	avgLen float64
}

// NewBM25Retriever 创建本地 BM25 检索器并预计算文档词频。
func NewBM25Retriever(docs []Result) *BM25Retriever {
	r := &BM25Retriever{docs: append([]Result(nil), docs...), df: map[string]int{}}
	totalLen := 0
	for _, doc := range r.docs {
		tokens := bm25Tokens(docText(doc))
		counts := map[string]int{}
		for _, token := range tokens {
			counts[token]++
		}
		r.tf = append(r.tf, counts)
		totalLen += len(tokens)
		seen := map[string]bool{}
		for token := range counts {
			if !seen[token] {
				r.df[token]++
				seen[token] = true
			}
		}
	}
	if len(r.docs) > 0 {
		r.avgLen = float64(totalLen) / float64(len(r.docs))
	}
	return r
}

func (r *BM25Retriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	tokens := uniqueTokens(bm25Tokens(q.Text))
	if len(tokens) == 0 || len(r.docs) == 0 {
		return nil, nil
	}
	k := q.K
	if k <= 0 {
		k = 5
	}
	type scored struct {
		result Result
		score  float64
	}
	var scoredDocs []scored
	excluded := queryStringSet(q.ExcludeIDs)
	for i, doc := range r.docs {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if _, ok := excluded[strings.ToLower(strings.TrimSpace(doc.ID))]; ok {
			continue
		}
		score := r.scoreDoc(tokens, i)
		if score <= 0 {
			continue
		}
		out := doc
		out.Score = score
		scoredDocs = append(scoredDocs, scored{result: out, score: score})
	}
	sort.SliceStable(scoredDocs, func(i, j int) bool {
		if scoredDocs[i].score == scoredDocs[j].score {
			return scoredDocs[i].result.ID < scoredDocs[j].result.ID
		}
		return scoredDocs[i].score > scoredDocs[j].score
	})
	if len(scoredDocs) > k {
		scoredDocs = scoredDocs[:k]
	}
	out := make([]Result, 0, len(scoredDocs))
	for _, item := range scoredDocs {
		out = append(out, item.result)
	}
	return out, nil
}

func queryStringSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func (r *BM25Retriever) scoreDoc(query []string, idx int) float64 {
	const k1 = 1.2
	const b = 0.75
	tf := r.tf[idx]
	docLen := 0
	for _, n := range tf {
		docLen += n
	}
	var score float64
	for _, token := range query {
		f := tf[token]
		if f == 0 {
			continue
		}
		df := r.df[token]
		idf := math.Log(1 + (float64(len(r.docs)-df)+0.5)/(float64(df)+0.5))
		denom := float64(f) + k1*(1-b+b*float64(docLen)/math.Max(r.avgLen, 1))
		score += idf * (float64(f) * (k1 + 1)) / denom
	}
	return score
}

func bm25Tokens(text string) []string {
	raw := bm25TokenRE.FindAllString(strings.ToLower(text), -1)
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if isHanToken(token) {
			runes := []rune(token)
			for _, r := range runes {
				out = append(out, string(r))
			}
			for i := 0; i+1 < len(runes); i++ {
				out = append(out, string(runes[i:i+2]))
			}
			continue
		}
		token = strings.Trim(token, ".,;:!?()[]{}\"'`")
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func uniqueTokens(tokens []string) []string {
	if len(tokens) < 2 {
		return tokens
	}
	seen := make(map[string]bool, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func isHanToken(token string) bool {
	for _, r := range token {
		if r < '\u4e00' || r > '\u9fff' {
			return false
		}
	}
	return true
}

func docText(doc Result) string {
	parts := []string{doc.Content, doc.Category}
	parts = append(parts, doc.Tags...)
	parts = append(parts, doc.ExpectedPoints...)
	return strings.Join(parts, " ")
}
