package questionbank

import (
	"strings"
	"unicode/utf8"
)

const (
	QualityFlagDirtyNoteMarker       = "dirty_note_marker"
	QualityFlagMultipleQuestionChain = "multiple_question_chain"
	QualityFlagContentTooLong        = "content_too_long"
	QualityFlagAnswerOrCommentLeak   = "answer_or_comment_leak"
	QualityFlagLowValueQuestion      = "low_value_question"
)

const maxQuestionContentRunes = 180

type ContentQualityResult struct {
	Flags         []string `json:"flags,omitempty"`
	HighRiskFlags []string `json:"high_risk_flags,omitempty"`
	AdvisoryFlags []string `json:"advisory_flags,omitempty"`
	HighRisk      bool     `json:"high_risk"`
}

func EvaluateQuestionContentQuality(content string) ContentQualityResult {
	text := strings.TrimSpace(content)
	lower := strings.ToLower(text)
	var flags []string

	if hasDirtyNoteMarker(text, lower) {
		flags = append(flags, QualityFlagDirtyNoteMarker)
	}
	if hasMultipleQuestionChain(text) {
		flags = append(flags, QualityFlagMultipleQuestionChain)
	}
	if utf8.RuneCountInString(text) > maxQuestionContentRunes {
		flags = append(flags, QualityFlagContentTooLong)
	}
	if hasAnswerOrCommentLeak(text, lower) {
		flags = append(flags, QualityFlagAnswerOrCommentLeak)
	}
	if isLowValueQuestion(text) {
		flags = append(flags, QualityFlagLowValueQuestion)
	}

	flags = compactStrings(flags)
	highRisk, advisory := splitQuestionContentQualityFlags(flags)
	return ContentQualityResult{
		Flags:         flags,
		HighRiskFlags: highRisk,
		AdvisoryFlags: advisory,
		HighRisk:      len(highRisk) > 0,
	}
}

func HasHighRiskQuestionContent(content string) bool {
	return EvaluateQuestionContentQuality(content).HighRisk
}

func hasDirtyNoteMarker(text, lower string) bool {
	if strings.Contains(text, "--") && (strings.Contains(text, "无法反驳") || strings.Contains(text, "不会答") || strings.Contains(text, "后面补")) {
		return true
	}
	markers := []string{
		"todo",
		"待补充",
		"后面补",
		"怎么说",
		"不会答",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) || strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func hasMultipleQuestionChain(text string) bool {
	if strings.Count(text, "？")+strings.Count(text, "?") >= 3 {
		return true
	}
	questionParticles := 0
	for _, marker := range []string{"吗", "么", "为什么", "怎么", "哪些", "是否"} {
		questionParticles += strings.Count(text, marker)
	}
	if questionParticles < 3 {
		return false
	}
	return strings.Contains(text, "--") ||
		strings.Count(text, "，") >= 2 ||
		strings.Contains(text, "  ")
}

func hasAnswerOrCommentLeak(text, lower string) bool {
	markers := []string{
		"无法反驳",
		"自我评价",
		"记忆方法",
		"完整回答",
		"针对我的项目怎么落地",
		"追问加分点",
		"答案：",
		"参考答案",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) || strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func splitQuestionContentQualityFlags(flags []string) ([]string, []string) {
	var highRisk []string
	var advisory []string
	for _, flag := range flags {
		switch flag {
		case QualityFlagDirtyNoteMarker, QualityFlagAnswerOrCommentLeak, QualityFlagLowValueQuestion:
			highRisk = append(highRisk, flag)
		case QualityFlagMultipleQuestionChain:
			if contains(flags, QualityFlagDirtyNoteMarker) || contains(flags, QualityFlagAnswerOrCommentLeak) {
				highRisk = append(highRisk, flag)
			} else {
				advisory = append(advisory, flag)
			}
		default:
			advisory = append(advisory, flag)
		}
	}
	return compactStrings(highRisk), compactStrings(advisory)
}
