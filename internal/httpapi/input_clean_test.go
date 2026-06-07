package httpapi

import (
	"strings"
	"testing"
)

func TestCleanStartInterviewRequestNormalizesAndRedactsInput(t *testing.T) {
	req := startInterviewRequest{
		SessionID:  "safe.session-01",
		JDText:     "岗位职责\r\n• 负责 Go 后端开发\t\t\n\n\n福利待遇\n五险一金\n投递方式\n邮箱 hr@example.com",
		ResumeText: "项目经历\r\n● 做过 PostgreSQL 优化\t手机号 13800138000\n微信 wechat_12345\n兴趣爱好\n篮球",
	}

	cleaned, err := cleanStartInterviewRequest(req)
	if err != nil {
		t.Fatalf("cleanStartInterviewRequest: %v", err)
	}
	if strings.Contains(cleaned.JDText, "\r") || strings.Contains(cleaned.ResumeText, "\r") {
		t.Fatalf("text should normalize CR newlines: jd=%q resume=%q", cleaned.JDText, cleaned.ResumeText)
	}
	if !strings.Contains(cleaned.JDText, "- 负责 Go 后端开发") {
		t.Fatalf("jd bullet not normalized: %q", cleaned.JDText)
	}
	if strings.Contains(cleaned.JDText, "福利待遇") || strings.Contains(cleaned.JDText, "投递方式") {
		t.Fatalf("jd low-value sections should be dropped: %q", cleaned.JDText)
	}
	if !strings.Contains(cleaned.JDText, "[REDACTED_EMAIL]") {
		t.Fatalf("jd email should be redacted before section drop or prompt use: %q", cleaned.JDText)
	}
	if strings.Contains(cleaned.ResumeText, "13800138000") || strings.Contains(cleaned.ResumeText, "wechat_12345") {
		t.Fatalf("resume pii should be redacted: %q", cleaned.ResumeText)
	}
	if !strings.Contains(cleaned.ResumeText, "[REDACTED_PHONE]") || !strings.Contains(cleaned.ResumeText, "[REDACTED_WECHAT]") {
		t.Fatalf("resume redaction markers missing: %q", cleaned.ResumeText)
	}
	if strings.Contains(cleaned.ResumeText, "兴趣爱好") {
		t.Fatalf("resume low-value section should be dropped: %q", cleaned.ResumeText)
	}
}

func TestCleanStartInterviewRequestRejectsInvalidSessionIDAndEmptyText(t *testing.T) {
	if _, err := cleanStartInterviewRequest(startInterviewRequest{
		SessionID:  "../bad",
		JDText:     "需要 Go 后端",
		ResumeText: "两年 Go 经验",
	}); err == nil {
		t.Fatal("expected invalid session_id to fail")
	}

	if _, err := cleanStartInterviewRequest(startInterviewRequest{
		SessionID:  "safe",
		JDText:     "福利待遇\n五险一金",
		ResumeText: "两年 Go 经验",
	}); err == nil {
		t.Fatal("expected jd cleaned to empty to fail")
	}
}

func TestCleanStartInterviewRequestRejectsOversizedText(t *testing.T) {
	if _, err := cleanStartInterviewRequest(startInterviewRequest{
		SessionID:  "safe",
		JDText:     strings.Repeat("a", maxJDTextRunes+1),
		ResumeText: "两年 Go 经验",
	}); err == nil {
		t.Fatal("expected oversized jd to fail")
	}
	if _, err := cleanStartInterviewRequest(startInterviewRequest{
		SessionID:  "safe",
		JDText:     "需要 Go 后端",
		ResumeText: strings.Repeat("a", maxResumeTextRunes+1),
	}); err == nil {
		t.Fatal("expected oversized resume to fail")
	}
}
