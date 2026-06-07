package httpapi

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	maxSessionIDRunes  = 128
	maxJDTextRunes     = 30_000
	maxResumeTextRunes = 50_000
)

var (
	sessionIDPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	emailPattern      = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phonePattern      = regexp.MustCompile(`(?:\+?86[- ]?)?1[3-9]\d{9}`)
	qqPattern         = regexp.MustCompile(`(?i)(?:qq[:：]?\s*)[1-9][0-9]{4,11}`)
	wechatPattern     = regexp.MustCompile(`(?i)(?:微信|wechat|weixin)[:：]?\s*[A-Za-z][A-Za-z0-9_-]{5,19}`)
	idCardPattern     = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	multiSpacePattern = regexp.MustCompile(`[ ]{2,}`)
)

func cleanStartInterviewRequest(req startInterviewRequest) (startInterviewRequest, error) {
	if err := validateOptionalSessionID(req.SessionID); err != nil {
		return req, err
	}

	jd, err := cleanInputText(req.JDText, maxJDTextRunes, jdLowValueLine)
	if err != nil {
		return req, fmt.Errorf("jd_text: %w", err)
	}
	resume, err := cleanInputText(req.ResumeText, maxResumeTextRunes, resumeLowValueLine)
	if err != nil {
		return req, fmt.Errorf("resume_text: %w", err)
	}
	req.JDText = jd
	req.ResumeText = resume
	req.UserID = strings.TrimSpace(req.UserID)
	req.Mode = strings.TrimSpace(req.Mode)
	return req, nil
}

func validateOptionalSessionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if len([]rune(id)) > maxSessionIDRunes {
		return fmt.Errorf("session_id too long")
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("session_id contains unsafe characters")
	}
	return nil
}

func cleanInputText(raw string, maxRunes int, lowValue func(string) bool) (string, error) {
	if len([]rune(raw)) > maxRunes {
		return "", fmt.Errorf("text exceeds %d characters", maxRunes)
	}
	text := normalizeInputText(raw)
	text = redactPII(text)
	text = filterInputLines(text, lowValue)
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("text is required")
	}
	return text, nil
}

func normalizeInputText(raw string) string {
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

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = multiSpacePattern.ReplaceAllString(strings.TrimSpace(line), " ")
	}
	return collapseBlankLines(lines)
}

func collapseBlankLines(lines []string) string {
	var out []string
	blank := 0
	for _, line := range lines {
		if line == "" {
			blank++
			if blank <= 2 {
				out = append(out, "")
			}
			continue
		}
		blank = 0
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func redactPII(text string) string {
	text = emailPattern.ReplaceAllString(text, "[REDACTED_EMAIL]")
	text = phonePattern.ReplaceAllString(text, "[REDACTED_PHONE]")
	text = qqPattern.ReplaceAllString(text, "[REDACTED_QQ]")
	text = wechatPattern.ReplaceAllString(text, "[REDACTED_WECHAT]")
	text = idCardPattern.ReplaceAllString(text, "[REDACTED_ID]")
	return text
}

func filterInputLines(text string, lowValue func(string) bool) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, "")
			continue
		}
		if lowValue != nil && lowValue(trimmed) {
			continue
		}
		out = append(out, trimmed)
	}
	return collapseBlankLines(out)
}

func jdLowValueLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	for _, key := range []string{
		"福利待遇", "公司介绍", "工作地点", "投递方式", "hr 联系", "hr联系", "招聘平台", "版权",
		"五险一金", "带薪年假", "下午茶", "团建",
	} {
		if strings.Contains(lower, strings.ToLower(key)) {
			return true
		}
	}
	return false
}

func resumeLowValueLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	for _, key := range []string{
		"兴趣爱好", "求职意向", "自我评价", "个人评价", "个人介绍",
	} {
		if strings.Contains(lower, strings.ToLower(key)) {
			return true
		}
	}
	return false
}
