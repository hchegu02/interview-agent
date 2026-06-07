package questionbank

import (
	"strings"
)

const (
	qualityFlagMissingContent       = "missing_content"
	qualityFlagMissingConcept       = "missing_concept"
	qualityFlagUnknownConcept       = "unknown_concept"
	qualityFlagMissingSourceRefs    = "missing_source_refs"
	qualityFlagUnknownSourceChunk   = "unknown_source_chunk"
	qualityFlagUngroundedQuote      = "ungrounded_quote"
	qualityFlagDuplicateContent         = "duplicate_content"
	qualityFlagDuplicateExistingContent = "duplicate_existing_content"
	qualityFlagLowValueQuestion     = "low_value_question"
	qualityFlagMissingFields        = "missing_required_fields"
	qualityFlagInvalidSingleChoice  = "invalid_single_choice"
	qualityFlagMissingFollowupHints = "missing_follow_up_hints"
	qualityFlagDifficultyMismatch   = "difficulty_mismatch"
)

func gateQuestionCandidates(req GenerationRequest, concepts []ConceptCard, chunks []RetrievedChunk, candidates []QuestionCandidate, existingContentKeys map[string]struct{}) ([]QuestionCandidate, []QuestionCandidate) {
	conceptsByID := make(map[string]ConceptCard, len(concepts))
	for _, concept := range concepts {
		conceptsByID[concept.ID] = concept
	}
	chunksByID := make(map[string]RetrievedChunk, len(chunks))
	for _, chunk := range chunks {
		chunksByID[chunk.ID] = chunk
	}

	seenContent := map[string]struct{}{}
	var passed []QuestionCandidate
	var rejected []QuestionCandidate
	for _, candidate := range candidates {
		flags := candidateQualityFlags(req, candidate, conceptsByID, chunksByID, seenContent, existingContentKeys)
		key := normalizeCandidateContent(candidate.Content)
		if key != "" {
			seenContent[key] = struct{}{}
		}
		if len(flags) > 0 {
			candidate.QualityFlags = append(candidate.QualityFlags, flags...)
			rejected = append(rejected, candidate)
			continue
		}
		passed = append(passed, candidate)
	}
	return passed, rejected
}

func candidateQualityFlags(req GenerationRequest, candidate QuestionCandidate, concepts map[string]ConceptCard, chunks map[string]RetrievedChunk, seenContent map[string]struct{}, existingContentKeys map[string]struct{}) []string {
	var flags []string
	if strings.TrimSpace(candidate.Content) == "" {
		flags = append(flags, qualityFlagMissingContent)
	}
	if missingCandidateRequiredFields(candidate) {
		flags = append(flags, qualityFlagMissingFields)
	}
	if isLowValueQuestion(candidate.Content) {
		flags = append(flags, qualityFlagLowValueQuestion)
	}
	key := normalizeCandidateContent(candidate.Content)
	if key != "" {
		if _, ok := seenContent[key]; ok {
			flags = append(flags, qualityFlagDuplicateContent)
		}
		if _, ok := existingContentKeys[key]; ok {
			flags = append(flags, qualityFlagDuplicateExistingContent)
		}
	}

	conceptID := strings.TrimSpace(candidate.ConceptID)
	if conceptID == "" {
		flags = append(flags, qualityFlagMissingConcept)
	} else {
		concept, ok := concepts[conceptID]
		if !ok {
			flags = append(flags, qualityFlagUnknownConcept)
		} else if concept.DifficultyHint > 0 && candidate.Difficulty > 0 && absInt(concept.DifficultyHint-candidate.Difficulty) > 2 {
			flags = append(flags, qualityFlagDifficultyMismatch)
		}
	}

	if len(candidate.SourceRefs) == 0 {
		flags = append(flags, qualityFlagMissingSourceRefs)
	} else {
		for _, ref := range candidate.SourceRefs {
			chunk, ok := chunks[ref.ChunkID]
			if !ok {
				flags = append(flags, qualityFlagUnknownSourceChunk)
				continue
			}
			quote := strings.TrimSpace(ref.Quote)
			if quote == "" || !strings.Contains(chunk.Content, quote) {
				flags = append(flags, qualityFlagUngroundedQuote)
			}
		}
	}

	questionType := candidate.QuestionType
	if questionType == "" {
		questionType = req.QuestionType
	}
	switch questionType {
	case "single_choice":
		if len(candidate.Options) < 4 || strings.TrimSpace(candidate.Answer) == "" {
			flags = append(flags, qualityFlagInvalidSingleChoice)
		}
	case "interview":
		if len(candidate.FollowUpHints) == 0 {
			flags = append(flags, qualityFlagMissingFollowupHints)
		}
	}
	return compactStrings(flags)
}

func missingCandidateRequiredFields(candidate QuestionCandidate) bool {
	return strings.TrimSpace(candidate.SkillCategory) == "" ||
		candidate.Difficulty < 1 || candidate.Difficulty > 5 ||
		len(candidate.Tags) == 0 ||
		len(candidate.ExpectedPoints) == 0 ||
		len(candidate.Rubric) == 0 ||
		strings.TrimSpace(candidate.SampleAnswer) == ""
}

func isLowValueQuestion(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	return normalized == "请总结本文" ||
		normalized == "总结本文" ||
		strings.Contains(normalized, "根据上文") ||
		strings.Contains(normalized, "总结上文")
}

func questionContentDedupeKey(content string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(content))), "")
}

func normalizeCandidateContent(content string) string {
	return questionContentDedupeKey(content)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
