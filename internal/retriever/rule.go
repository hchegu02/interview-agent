package retriever

import (
	"context"
	"sort"
	"strings"
)

type RuleRetriever struct {
	docs []Result
}

func NewRuleRetriever(docs []Result) *RuleRetriever {
	return &RuleRetriever{docs: append([]Result(nil), docs...)}
}

func (r *RuleRetriever) Retrieve(ctx context.Context, q Query) ([]Result, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	k := q.K
	if k <= 0 {
		k = 5
	}
	var out []Result
	for _, doc := range r.docs {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !ruleMatchesHardFilters(doc, q) {
			continue
		}
		score := ruleScore(doc, q)
		if score <= 0 {
			continue
		}
		item := doc
		item.Score = score
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func ruleMatchesHardFilters(doc Result, q Query) bool {
	if len(q.SkillCategories) > 0 && !ruleContainsFold(q.SkillCategories, doc.Category) {
		return false
	}
	if len(q.FilterTags) > 0 && !ruleTagsOverlap(doc.Tags, q.FilterTags) {
		return false
	}
	if q.DifficultyMin > 0 && doc.Difficulty < q.DifficultyMin {
		return false
	}
	if q.DifficultyMax > 0 && doc.Difficulty > q.DifficultyMax {
		return false
	}
	return true
}

func ruleScore(doc Result, q Query) float64 {
	score := 0.0
	if ruleHasHardFilters(q) {
		score = 1
	} else if ruleHasSoftSignals(q) {
		score = 0.1
	} else {
		return 0
	}
	if len(q.Tags) > 0 {
		for _, tag := range doc.Tags {
			if ruleContainsFold(q.Tags, tag) {
				score += 1
			}
		}
	}
	if q.Difficulty > 0 && doc.Difficulty == q.Difficulty {
		score += 0.5
	}
	return score
}

func ruleHasHardFilters(q Query) bool {
	return len(q.SkillCategories) > 0 || len(q.FilterTags) > 0 || q.DifficultyMin > 0 || q.DifficultyMax > 0
}

func ruleHasSoftSignals(q Query) bool {
	return ruleHasNonEmpty(q.Tags) || q.Difficulty > 0
}

func ruleHasNonEmpty(items []string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return true
		}
	}
	return false
}

func ruleTagsOverlap(tags []string, filters []string) bool {
	for _, tag := range tags {
		if ruleContainsFold(filters, tag) {
			return true
		}
	}
	return false
}

func ruleContainsFold(items []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}
