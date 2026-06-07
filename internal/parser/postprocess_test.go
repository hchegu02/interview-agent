package parser

import (
	"strings"
	"testing"
)

func TestPostProcessTextRemovesPageArtifactsAndNormalizesBullets(t *testing.T) {
	raw := "简历标题\r\nPage 1 of 3\n• Go 后端开发\n- 2 -\n● Redis 缓存优化\n3\n"

	got := PostProcessText(raw, PostProcessOptions{Kind: "resume"}).Text

	for _, forbidden := range []string{"Page 1 of 3", "- 2 -", "\r", "●", "•"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("postprocessed text still contains %q: %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "- Go 后端开发") || !strings.Contains(got, "- Redis 缓存优化") {
		t.Fatalf("bullets not normalized: %q", got)
	}
}

func TestPostProcessTextDropsRepeatedShortHeaders(t *testing.T) {
	raw := strings.Join([]string{
		"张三的简历",
		"项目经历",
		"负责 Go 服务开发。",
		"张三的简历",
		"实习经历",
		"参与 Redis 缓存治理。",
		"张三的简历",
	}, "\n")

	got := PostProcessText(raw, PostProcessOptions{Kind: "resume"}).Text

	if strings.Contains(got, "张三的简历") {
		t.Fatalf("repeated header should be removed: %q", got)
	}
	if !strings.Contains(got, "负责 Go 服务开发") || !strings.Contains(got, "参与 Redis 缓存治理") {
		t.Fatalf("useful content lost: %q", got)
	}
}

func TestPostProcessTextMergesPDFHardLineBreaks(t *testing.T) {
	raw := "负责 Go 微服务\n高并发接口开发。\n- 使用 Redis 做缓存\n- 使用 PostgreSQL 优化查询\n"

	got := PostProcessText(raw, PostProcessOptions{Kind: "resume"}).Text

	if !strings.Contains(got, "负责 Go 微服务 高并发接口开发。") {
		t.Fatalf("hard line break not merged: %q", got)
	}
	if !strings.Contains(got, "- 使用 Redis 做缓存\n- 使用 PostgreSQL 优化查询") {
		t.Fatalf("list item boundaries should be preserved: %q", got)
	}
}

func TestPostProcessTextSplitsVeryLongParagraph(t *testing.T) {
	raw := strings.Repeat("负责 Go 服务开发，", 180)

	got := PostProcessText(raw, PostProcessOptions{Kind: "resume"}).Text

	parts := strings.Split(got, "\n")
	if len(parts) < 2 {
		t.Fatalf("long paragraph should be split, got one paragraph length=%d", len(got))
	}
	for _, part := range parts {
		if len([]rune(part)) > 900 {
			t.Fatalf("split paragraph still too long: %d runes", len([]rune(part)))
		}
	}
}
