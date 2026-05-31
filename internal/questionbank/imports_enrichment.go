package questionbank

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"interview-agent/internal/llm"
)

func (s *ImportService) generateItems(ctx context.Context, chunk string) ([]Item, error) {
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是题库生成助手。只输出 JSON。"},
		{Role: "user", Content: "从下面的工程实践文档切片中生成 3-5 道后端面试题，字段为 items 数组，每项包含 id, content, tags, skill_category, difficulty, expected_points, rubric, sample_answer, follow_up_hints。\n\n" + chunk},
	}, llm.Options{MaxTokens: 1600, Temperature: 0.2}, validateItemsJSON, 1)
	if err != nil {
		return nil, err
	}
	return parseQuestionBankItems("generated.json", []byte(resp.Content))
}

func (s *ImportService) enrichLocalItems(ctx context.Context, items []Item) ([]Item, []map[string]string, error) {
	provenances := make([]map[string]string, len(items))
	if s == nil || s.model == nil || len(items) == 0 {
		return items, provenances, nil
	}

	need := make([]Item, 0, len(items))
	for _, item := range items {
		if needsEnrichment(item) {
			need = append(need, item)
		}
	}
	if len(need) == 0 {
		return items, provenances, nil
	}

	enriched := make([]Item, 0, len(need))
	for start := 0; start < len(need); start += localEnrichmentBatchSize {
		end := start + localEnrichmentBatchSize
		if end > len(need) {
			end = len(need)
		}
		batch, err := s.enrichLocalBatch(ctx, need[start:end])
		if err != nil {
			return nil, nil, err
		}
		enriched = append(enriched, batch...)
	}
	byID := make(map[string]Item, len(enriched))
	byContent := make(map[string]Item, len(enriched))
	for _, item := range enriched {
		if id := strings.TrimSpace(item.ID); id != "" {
			byID[id] = item
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			byContent[content] = item
		}
	}

	out := make([]Item, 0, len(items))
	for _, item := range items {
		if !needsEnrichment(item) {
			out = append(out, item)
			continue
		}
		enrichedItem, ok := byID[strings.TrimSpace(item.ID)]
		if !ok {
			enrichedItem, ok = byContent[strings.TrimSpace(item.Content)]
		}
		if !ok {
			return nil, nil, fmt.Errorf("llm enrichment missing item %q", itemIdentity(item))
		}
		var fieldProvenance map[string]string
		item, fieldProvenance = mergeEnrichedItemWithProvenance(item, enrichedItem)
		provenances[len(out)] = fieldProvenance
		out = append(out, item)
	}
	return out, provenances, nil
}

func (s *ImportService) enrichLocalBatch(ctx context.Context, items []Item) ([]Item, error) {
	raw, err := json.Marshal(struct {
		Items []Item `json:"items"`
	}{Items: items})
	if err != nil {
		return nil, err
	}
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是题库元数据补全助手。只输出 JSON。"},
		{Role: "user", Content: "补齐题库元数据。保留每道题的 id 和 content，返回 items 数组；每项补齐 tags, skill_category, difficulty, expected_points, rubric, sample_answer, follow_up_hints。必须为输入中的每一道题返回一项，不能新增题目，不能漏题。\n\n" + string(raw)},
	}, llm.Options{MaxTokens: 1800, Temperature: 0.2}, validateItemsJSON, 1)
	if err != nil {
		return nil, err
	}
	enriched, err := parseQuestionBankItems("enriched.json", []byte(resp.Content))
	if err != nil {
		return nil, err
	}
	if err := validateEnrichmentCoverage(items, enriched); err != nil {
		return nil, err
	}
	return enriched, nil
}

func validateItemsJSON(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	items, err := parseJSONItems(raw)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return errors.New("items must not be empty")
	}
	return nil
}

func needsEnrichment(item Item) bool {
	return strings.TrimSpace(item.SkillCategory) == "" ||
		strings.TrimSpace(item.SkillCategory) == "general" ||
		item.Difficulty == 0 ||
		len(item.Tags) == 0 ||
		len(item.ExpectedPoints) == 0 ||
		len(item.Rubric) == 0 ||
		strings.TrimSpace(item.SampleAnswer) == "" ||
		len(item.FollowUpHints) == 0
}

func mergeEnrichedItem(base, enriched Item) Item {
	merged, _ := mergeEnrichedItemWithProvenance(base, enriched)
	return merged
}

func mergeEnrichedItemWithProvenance(base, enriched Item) (Item, map[string]string) {
	provenance := map[string]string{}
	if strings.TrimSpace(base.SkillCategory) == "" || strings.TrimSpace(base.SkillCategory) == "general" {
		if strings.TrimSpace(enriched.SkillCategory) != "" {
			if strings.TrimSpace(base.SkillCategory) == "general" {
				provenance["skill_category"] = "merged"
			} else {
				provenance["skill_category"] = "llm"
			}
		}
		base.SkillCategory = enriched.SkillCategory
	}
	if base.Difficulty == 0 {
		if enriched.Difficulty != 0 {
			provenance["difficulty"] = "llm"
		}
		base.Difficulty = enriched.Difficulty
	}
	if len(base.Tags) == 0 {
		if len(enriched.Tags) > 0 {
			provenance["tags"] = "llm"
		}
		base.Tags = enriched.Tags
	}
	if len(base.ExpectedPoints) == 0 {
		if len(enriched.ExpectedPoints) > 0 {
			provenance["expected_points"] = "llm"
		}
		base.ExpectedPoints = enriched.ExpectedPoints
	}
	if len(base.Rubric) == 0 {
		if len(enriched.Rubric) > 0 {
			provenance["rubric"] = "llm"
		}
		base.Rubric = enriched.Rubric
	}
	if strings.TrimSpace(base.SampleAnswer) == "" {
		if strings.TrimSpace(enriched.SampleAnswer) != "" {
			provenance["sample_answer"] = "llm"
		}
		base.SampleAnswer = enriched.SampleAnswer
	}
	if len(base.FollowUpHints) == 0 {
		if len(enriched.FollowUpHints) > 0 {
			provenance["follow_up_hints"] = "llm"
		}
		base.FollowUpHints = enriched.FollowUpHints
	}
	return base, provenance
}

func importFieldProvenance(parsed Item, normalized Item, original *Item) map[string]string {
	provenance := map[string]string{}
	mark := func(field string, uploaded bool, defaulted bool) {
		switch {
		case uploaded:
			provenance[field] = "uploaded"
		case defaulted:
			provenance[field] = "default"
		}
	}
	if original == nil {
		mark("skill_category", strings.TrimSpace(parsed.SkillCategory) != "", strings.TrimSpace(normalized.SkillCategory) == "general")
		mark("difficulty", parsed.Difficulty != 0, normalized.Difficulty == 3)
		mark("tags", len(parsed.Tags) > 0, false)
		mark("expected_points", len(parsed.ExpectedPoints) > 0, false)
		mark("rubric", len(parsed.Rubric) > 0, false)
		mark("sample_answer", strings.TrimSpace(parsed.SampleAnswer) != "", false)
		mark("follow_up_hints", len(parsed.FollowUpHints) > 0, false)
		for field, source := range provenance {
			if source == "uploaded" {
				provenance[field] = "generated"
			}
		}
		return provenance
	}
	mark("skill_category", strings.TrimSpace(original.SkillCategory) != "", strings.TrimSpace(normalized.SkillCategory) == "general")
	mark("difficulty", original.Difficulty != 0, normalized.Difficulty == 3)
	mark("tags", len(original.Tags) > 0, false)
	mark("expected_points", len(original.ExpectedPoints) > 0, false)
	mark("rubric", len(original.Rubric) > 0, false)
	mark("sample_answer", strings.TrimSpace(original.SampleAnswer) != "", false)
	mark("follow_up_hints", len(original.FollowUpHints) > 0, false)
	return provenance
}

func validateEnrichmentCoverage(inputs, enriched []Item) error {
	byID := make(map[string]struct{}, len(enriched))
	byContent := make(map[string]struct{}, len(enriched))
	for _, item := range enriched {
		if id := strings.TrimSpace(item.ID); id != "" {
			byID[id] = struct{}{}
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			byContent[content] = struct{}{}
		}
	}
	for _, item := range inputs {
		if id := strings.TrimSpace(item.ID); id != "" {
			if _, ok := byID[id]; ok {
				continue
			}
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			if _, ok := byContent[content]; ok {
				continue
			}
		}
		return fmt.Errorf("llm enrichment missing item %q", itemIdentity(item))
	}
	return nil
}

func itemIdentity(item Item) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return strings.TrimSpace(item.Content)
}
