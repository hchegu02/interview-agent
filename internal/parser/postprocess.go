package parser

import (
	"regexp"
	"strings"
	"unicode"
)

const maxPostProcessParagraphRunes = 800

type PostProcessOptions struct {
	Kind string
}

type ProcessedText struct {
	Text string
}

var (
	pageNumberLinePattern       = regexp.MustCompile(`(?i)^\s*(?:-?\s*\d+\s*-?|page\s+\d+(?:\s+of\s+\d+)?)\s*$`)
	postProcessMultiSpaceRegexp = regexp.MustCompile(`[ ]{2,}`)
)

func PostProcessText(raw string, opts PostProcessOptions) ProcessedText {
	_ = opts
	lines := normalizePostProcessLines(raw)
	lines = dropPageNumberLines(lines)
	lines = dropRepeatedShortLines(lines)
	lines = mergeHardLineBreaks(lines)
	lines = splitLongParagraphs(lines, maxPostProcessParagraphRunes)
	text := collapsePostProcessBlankLines(lines)
	return ProcessedText{Text: text}
}

func normalizePostProcessLines(raw string) []string {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff':
			return -1
		case '•', '●', '·', '–', '—', '▪':
			return '-'
		}
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, text)
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = postProcessMultiSpaceRegexp.ReplaceAllString(strings.TrimSpace(line), " ")
		lines = append(lines, line)
	}
	return lines
}

func dropPageNumberLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if pageNumberLinePattern.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func dropRepeatedShortLines(lines []string) []string {
	counts := map[string]int{}
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" || len([]rune(key)) > 24 || likelyTechnicalLine(key) {
			continue
		}
		counts[key]++
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if counts[key] >= 3 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func likelyTechnicalLine(line string) bool {
	lower := strings.ToLower(line)
	for _, token := range []string{"go", "redis", "mysql", "postgres", "postgresql", "kafka", "http", "grpc", "docker", "kubernetes", "k8s", "llm", "rag"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func mergeHardLineBreaks(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		if len(out) == 0 || shouldStartNewParagraph(line) || shouldKeepPreviousBoundary(out[len(out)-1]) {
			out = append(out, line)
			continue
		}
		out[len(out)-1] = out[len(out)-1] + " " + line
	}
	return out
}

func shouldStartNewParagraph(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- ") ||
		strings.HasSuffix(trimmed, "：") ||
		strings.HasSuffix(trimmed, ":") ||
		isLikelySectionHeading(trimmed)
}

func shouldKeepPreviousBoundary(prev string) bool {
	prev = strings.TrimSpace(prev)
	return prev == "" ||
		strings.HasPrefix(prev, "- ") ||
		isLikelySectionHeading(prev) ||
		strings.HasSuffix(prev, "。") ||
		strings.HasSuffix(prev, ".") ||
		strings.HasSuffix(prev, "；") ||
		strings.HasSuffix(prev, ";") ||
		strings.HasSuffix(prev, "：") ||
		strings.HasSuffix(prev, ":")
}

func isLikelySectionHeading(line string) bool {
	switch strings.TrimSpace(line) {
	case "项目经历", "实习经历", "工作经历", "专业技能", "教育经历", "个人信息", "获奖经历", "开源项目":
		return true
	default:
		return false
	}
}

func splitLongParagraphs(lines []string, maxRunes int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if len([]rune(line)) <= maxRunes {
			out = append(out, line)
			continue
		}
		out = append(out, splitLongLine(line, maxRunes)...)
	}
	return out
}

func splitLongLine(line string, maxRunes int) []string {
	var out []string
	var current strings.Builder
	count := 0
	for _, r := range line {
		current.WriteRune(r)
		count++
		if count >= maxRunes || r == '。' || r == '；' || r == ';' {
			part := strings.TrimSpace(current.String())
			if part != "" {
				out = append(out, part)
			}
			current.Reset()
			count = 0
		}
	}
	if rest := strings.TrimSpace(current.String()); rest != "" {
		out = append(out, rest)
	}
	return out
}

func collapsePostProcessBlankLines(lines []string) string {
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(line))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
