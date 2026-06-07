package questionbank

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"interview-agent/internal/llm"
)

func (s *GenerationService) extractConceptCards(ctx context.Context, req GenerationRequest, chunks []RetrievedChunk) ([]ConceptCard, []string, error) {
	if len(chunks) == 0 {
		return nil, nil, nil
	}
	var cards []ConceptCard
	if s == nil || s.model == nil {
		cards = fallbackConceptCards(req, chunks)
	} else {
		generated, err := s.generateConceptCards(ctx, req, chunks)
		if err != nil {
			return nil, nil, err
		}
		cards = generated
	}
	valid, rejected := validateConceptCards(importGeneratedID("gen", req.SourceJobID+":"+req.Topic), cards, chunks)
	return valid, rejected, nil
}

func (s *GenerationService) generateConceptCards(ctx context.Context, req GenerationRequest, chunks []RetrievedChunk) ([]ConceptCard, error) {
	raw, err := json.Marshal(struct {
		Request GenerationRequest `json:"request"`
		Chunks  []RetrievedChunk  `json:"chunks"`
	}{Request: req, Chunks: chunks})
	if err != nil {
		return nil, err
	}
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是题库能力点抽取助手。只输出 JSON。"},
		{Role: "user", Content: "从 evidence chunks 中抽取可出题的 concept cards。返回 JSON: {\"concepts\":[{\"title\":\"\",\"skill\":\"\",\"sub_skill\":\"\",\"difficulty_hint\":3,\"keywords\":[],\"question_angles\":[],\"evidence_refs\":[{\"chunk_id\":\"\",\"quote\":\"\"}]}]}。\n\n" + string(raw)},
	}, llm.Options{MaxTokens: 1200, Temperature: 0.1}, validateConceptCardsJSON, 1)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Concepts []ConceptCard `json:"concepts"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, err
	}
	return parsed.Concepts, nil
}

func validateConceptCardsJSON(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	var parsed struct {
		Concepts []ConceptCard `json:"concepts"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if len(parsed.Concepts) == 0 {
		return fmt.Errorf("concepts must not be empty")
	}
	return nil
}

func parseQuestionCandidatesJSON(raw []byte) ([]QuestionCandidate, error) {
	if err := validateQuestionCandidatesJSON(raw); err != nil {
		return nil, err
	}
	var parsed struct {
		Candidates []QuestionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return parsed.Candidates, nil
}

func validateQuestionCandidatesJSON(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	var parsed struct {
		Candidates []QuestionCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if len(parsed.Candidates) == 0 {
		return fmt.Errorf("candidates must not be empty")
	}
	return nil
}

func (s *GenerationService) generateQuestionCandidates(ctx context.Context, req GenerationRequest, concepts []ConceptCard, chunks []RetrievedChunk) ([]QuestionCandidate, error) {
	if s == nil || s.model == nil {
		return nil, fmt.Errorf("question candidate generation requires llm model")
	}
	raw, err := json.Marshal(struct {
		Request  GenerationRequest `json:"request"`
		Concepts []ConceptCard     `json:"concepts"`
		Chunks   []RetrievedChunk  `json:"chunks"`
	}{Request: req, Concepts: concepts, Chunks: chunks})
	if err != nil {
		return nil, err
	}
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是证据约束的面试题生成助手。只输出 JSON，不要输出 markdown。"},
		{Role: "user", Content: "基于 concept cards 和 evidence chunks 生成候选题。要求每题必须引用 concept_id 和 source_refs，source_refs.quote 必须是 chunks 原文子串。返回 JSON: {\"candidates\":[{\"concept_id\":\"\",\"content\":\"\",\"question_type\":\"interview|single_choice|short_answer\",\"target_dimension\":\"\",\"options\":[],\"answer\":\"\",\"explanation\":\"\",\"tags\":[],\"skill_category\":\"\",\"difficulty\":3,\"expected_points\":[],\"rubric\":{\"good\":\"\"},\"sample_answer\":\"\",\"follow_up_hints\":[],\"source_refs\":[{\"chunk_id\":\"\",\"quote\":\"\"}]}]}。\n\n" + string(raw)},
	}, llm.Options{MaxTokens: 1800, Temperature: 0.2}, validateQuestionCandidatesJSON, 1)
	if err != nil {
		return nil, err
	}
	return parseQuestionCandidatesJSON([]byte(resp.Content))
}

func fallbackConceptCards(req GenerationRequest, chunks []RetrievedChunk) []ConceptCard {
	if len(chunks) == 0 {
		return nil
	}
	title := strings.TrimSpace(req.Topic)
	if title == "" {
		title = "source concept"
	}
	chunk := chunks[0]
	return []ConceptCard{{
		Title:          title,
		Skill:          strings.TrimSpace(req.SkillCategory),
		DifficultyHint: req.Difficulty,
		Keywords:       generationRetrievalTerms(req),
		QuestionAngles: compactStrings([]string{req.TargetDimension, req.QuestionType}),
		EvidenceRefs: []SourceRef{{
			ChunkID: chunk.ID,
			Quote:   generationEvidenceQuote(chunk.Content),
		}},
	}}
}

func generationEvidenceQuote(content string) string {
	content = strings.TrimSpace(content)
	runes := []rune(content)
	if len(runes) <= 80 {
		return content
	}
	return string(runes[:80])
}

func validateConceptCards(generationJobID string, cards []ConceptCard, chunks []RetrievedChunk) ([]ConceptCard, []string) {
	chunksByID := make(map[string]RetrievedChunk, len(chunks))
	for _, chunk := range chunks {
		chunksByID[chunk.ID] = chunk
	}

	seen := map[string]struct{}{}
	valid := make([]ConceptCard, 0, len(cards))
	var rejected []string
	for i, card := range cards {
		card.Title = strings.TrimSpace(card.Title)
		if card.Title == "" {
			rejected = append(rejected, "concept title is required")
			continue
		}
		if len(card.EvidenceRefs) == 0 {
			rejected = append(rejected, fmt.Sprintf("concept %q missing evidence_refs", card.Title))
			continue
		}
		if reason := validateConceptEvidence(card, chunksByID); reason != "" {
			rejected = append(rejected, reason)
			continue
		}
		key := conceptDedupKey(card)
		if _, ok := seen[key]; ok {
			rejected = append(rejected, fmt.Sprintf("duplicate concept %q", card.Title))
			continue
		}
		seen[key] = struct{}{}
		card.ID = conceptID(generationJobID, i, card)
		valid = append(valid, card)
	}
	return valid, rejected
}

func validateConceptEvidence(card ConceptCard, chunksByID map[string]RetrievedChunk) string {
	for _, ref := range card.EvidenceRefs {
		chunk, ok := chunksByID[ref.ChunkID]
		if !ok {
			return fmt.Sprintf("concept %q references unknown chunk %q", card.Title, ref.ChunkID)
		}
		quote := strings.TrimSpace(ref.Quote)
		if quote == "" {
			return fmt.Sprintf("concept %q has empty evidence quote", card.Title)
		}
		if !strings.Contains(chunk.Content, quote) {
			return fmt.Sprintf("concept %q quote is not grounded in chunk %q", card.Title, ref.ChunkID)
		}
	}
	return ""
}

func conceptID(generationJobID string, index int, card ConceptCard) string {
	return importGeneratedID("concept", fmt.Sprintf("%s:%03d:%s:%s", generationJobID, index, card.Title, conceptRefsFingerprint(card.EvidenceRefs)))
}

func conceptDedupKey(card ConceptCard) string {
	return strings.ToLower(strings.TrimSpace(card.Title)) + ":" + conceptRefsFingerprint(card.EvidenceRefs)
}

func conceptRefsFingerprint(refs []SourceRef) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, strings.TrimSpace(ref.ChunkID)+"="+strings.TrimSpace(ref.Quote))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
