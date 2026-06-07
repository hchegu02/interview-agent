package questionbank

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

type RetrievedChunk struct {
	ID      string  `json:"chunk_id"`
	JobID   string  `json:"job_id"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func retrieveGenerationChunks(ctx context.Context, store ImportStore, req GenerationRequest, limit int) ([]RetrievedChunk, error) {
	if store == nil {
		return nil, errorsNotConfigured()
	}
	if limit <= 0 {
		limit = 8
	}
	chunks, err := store.ListChunks(ctx, req.SourceJobID)
	if err != nil {
		return nil, err
	}
	terms := generationRetrievalTerms(req)
	if len(terms) == 0 {
		return nil, nil
	}
	out := make([]RetrievedChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := scoreGenerationChunk(chunk.Content, terms)
		if score <= 0 {
			continue
		}
		out = append(out, RetrievedChunk{
			ID:      chunk.ID,
			JobID:   chunk.JobID,
			Content: chunk.Content,
			Score:   score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func generationRetrievalTerms(req GenerationRequest) []string {
	var raw []string
	raw = append(raw, req.Topic, req.SkillCategory, req.QuestionType, req.TargetDimension)
	raw = append(raw, req.Tags...)
	seen := map[string]struct{}{}
	var out []string
	for _, term := range raw {
		for _, part := range splitGenerationTerms(term) {
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return out
}

func splitGenerationTerms(raw string) []string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(raw)), func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '，' || r == ';' || r == '；' || r == '/' || r == '\\'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || len([]rune(field)) < 2 {
			continue
		}
		out = append(out, field)
	}
	return out
}

func scoreGenerationChunk(content string, terms []string) float64 {
	content = strings.ToLower(content)
	var score float64
	for _, term := range terms {
		if strings.Contains(content, term) {
			score += 1
		}
	}
	return score
}
