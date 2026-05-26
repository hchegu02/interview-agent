// cmd/demo 写 run.json + report.md 的输出层。
//
// 两份产物的分工：
//   - run.json：机器可读，含 Session、所有 LLM CallRecord、所有 NodeRecord、
//     汇总统计。给后续做 prompt regression diff / token 用量分析的工具读。
//   - report.md：人读摘要，给操作者快速判断"这轮跑得怎么样"——5 段：
//     Run config / Pipeline timeline / LLM call stats / Schema 错分布 / Final report。
//
// 写盘前 MkdirAll 兜底；output 目录不存在不影响 demo 主流程。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/llm"
	"interview-agent/internal/observability"
)

// RunArtifact 是 run.json 的顶层结构。
// 字段顺序与可读性优先——StartedAt / EndedAt 放最前面方便 grep。
type RunArtifact struct {
	StartedAt         time.Time            `json:"started_at"`
	EndedAt           time.Time            `json:"ended_at"`
	Config            RunConfig            `json:"config"`
	Session           *domain.Session      `json:"session,omitempty"`
	LLMCalls          []llm.CallRecord     `json:"llm_calls"`
	Nodes             []observability.NodeRecord `json:"nodes"`
	BreakerStateFinal string               `json:"breaker_state_final,omitempty"`
	Summary           RunSummary           `json:"summary"`
	FatalError        string               `json:"fatal_error,omitempty"`
}

// RunConfig 是 demo 运行时的元数据，仅含非敏感信息。
// 严禁含 API key / DSN 等——run.json 可能被发到 issue tracker。
type RunConfig struct {
	LLMMode       string `json:"llm_mode"`
	LLMModel      string `json:"llm_model,omitempty"`
	EmbeddingMode string `json:"embedding_mode"`
	ScriptPath    string `json:"script_path"`
	OutputDir     string `json:"output_dir"`
}

// RunSummary 是给操作者扫一眼就懂的关键指标。
type RunSummary struct {
	TotalDurationMS       int64 `json:"total_duration_ms"`
	TotalLLMCalls         int   `json:"total_llm_calls"`
	TotalPromptTokens     int   `json:"total_prompt_tokens"`
	TotalCompletionTokens int   `json:"total_completion_tokens"`
	// SchemaRetries 是 CallWithSchema 自纠正循环触发的次数（grep slog 计数）。
	// 这里从 LLMCalls 推不出来——一次 schema 通过对应一次 Generate 但失败也算一次，
	// CLI 在 LLM 调用之外另外统计（见 main.go 的 slog handler）。
	SchemaRetries int `json:"schema_retries"`
	// 按 llm.ClassifyChatErr 桶计数；ok 不在此表。
	ErrorBuckets map[string]int `json:"error_buckets"`
	Rounds       int            `json:"rounds"`
	// 报告最终总分；session 未完成时为 -1。
	ReportOverallScore int `json:"report_overall_score"`
}

// WriteRunArtifact 把 RunArtifact 写到 {dir}/run.json。
// json.MarshalIndent 给人读：两格缩进，HTML escape off 避免 < > 被转义。
func WriteRunArtifact(dir string, art *RunArtifact) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	f, err := os.Create(filepath.Join(dir, "run.json"))
	if err != nil {
		return fmt.Errorf("create run.json: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(art); err != nil {
		return fmt.Errorf("encode run.json: %w", err)
	}
	return nil
}

// WriteReportMarkdown 写人读摘要 {dir}/report.md。
// 排版尽量 grep 友好——节点 / 调用都列成表，方便复制到 issue 评论。
func WriteReportMarkdown(dir string, art *RunArtifact) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	var b strings.Builder
	b.WriteString("# Interview Agent Demo Run\n\n")
	fmt.Fprintf(&b, "- started: `%s`\n", art.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- ended:   `%s`\n", art.EndedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- duration: %d ms\n", art.Summary.TotalDurationMS)
	if art.FatalError != "" {
		fmt.Fprintf(&b, "- **fatal error**: `%s`\n", art.FatalError)
	}
	b.WriteString("\n## Run config\n\n")
	fmt.Fprintf(&b, "- llm.mode: `%s`\n", art.Config.LLMMode)
	if art.Config.LLMModel != "" {
		fmt.Fprintf(&b, "- llm.model: `%s`\n", art.Config.LLMModel)
	}
	fmt.Fprintf(&b, "- embedding.mode: `%s`\n", art.Config.EmbeddingMode)
	fmt.Fprintf(&b, "- script: `%s`\n", art.Config.ScriptPath)
	fmt.Fprintf(&b, "- output dir: `%s`\n", art.Config.OutputDir)
	if art.BreakerStateFinal != "" {
		fmt.Fprintf(&b, "- breaker state (final): `%s`\n", art.BreakerStateFinal)
	}

	b.WriteString("\n## Pipeline timeline\n\n")
	if len(art.Nodes) == 0 {
		b.WriteString("_no node records_\n")
	} else {
		b.WriteString("| # | node | duration ms | err class | err msg |\n")
		b.WriteString("|---|------|-------------|-----------|---------|\n")
		for i, n := range art.Nodes {
			fmt.Fprintf(&b, "| %d | %s | %d | %s | %s |\n",
				i+1, n.Node, n.Duration.Milliseconds(), n.ErrClass, escapeMD(n.ErrMsg))
		}
	}

	b.WriteString("\n## LLM call stats\n\n")
	fmt.Fprintf(&b, "- total calls: %d\n", art.Summary.TotalLLMCalls)
	fmt.Fprintf(&b, "- total prompt tokens: %d\n", art.Summary.TotalPromptTokens)
	fmt.Fprintf(&b, "- total completion tokens: %d\n", art.Summary.TotalCompletionTokens)
	fmt.Fprintf(&b, "- schema retries (self-correction triggered): %d\n", art.Summary.SchemaRetries)

	b.WriteString("\n## Error breakdown\n\n")
	if len(art.Summary.ErrorBuckets) == 0 {
		b.WriteString("_no errors_\n")
	} else {
		b.WriteString("| bucket | count |\n|--------|-------|\n")
		keys := make([]string, 0, len(art.Summary.ErrorBuckets))
		for k := range art.Summary.ErrorBuckets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "| %s | %d |\n", k, art.Summary.ErrorBuckets[k])
		}
	}

	b.WriteString("\n## Final report\n\n")
	if art.Session != nil && art.Session.Report != nil {
		rep := art.Session.Report
		fmt.Fprintf(&b, "- overall score: **%d**\n", rep.OverallScore)
		fmt.Fprintf(&b, "- rounds asked: %d\n", art.Summary.Rounds)
		if len(rep.Highlights) > 0 {
			b.WriteString("\n### Highlights\n")
			for _, h := range rep.Highlights {
				fmt.Fprintf(&b, "- %s\n", h)
			}
		}
		if len(rep.Improvements) > 0 {
			b.WriteString("\n### Improvements\n")
			for _, h := range rep.Improvements {
				fmt.Fprintf(&b, "- %s\n", h)
			}
		}
		if len(rep.NextSteps) > 0 {
			b.WriteString("\n### Next steps\n")
			for _, h := range rep.NextSteps {
				fmt.Fprintf(&b, "- %s\n", h)
			}
		}
		if len(rep.SkillBreakdown) > 0 {
			b.WriteString("\n### Skill breakdown\n\n")
			b.WriteString("| skill | score |\n|-------|-------|\n")
			skills := make([]string, 0, len(rep.SkillBreakdown))
			for k := range rep.SkillBreakdown {
				skills = append(skills, k)
			}
			sort.Strings(skills)
			for _, k := range skills {
				fmt.Fprintf(&b, "| %s | %d |\n", k, rep.SkillBreakdown[k])
			}
		}
	} else if art.Session != nil {
		fmt.Fprintf(&b, "_session ended in status `%s` without a report._\n", art.Session.Status)
	} else {
		b.WriteString("_no session captured_\n")
	}

	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write report.md: %w", err)
	}
	return nil
}

// BuildSummary 从已采集的 records 聚合关键指标。
// breakerStateFinal 由调用方注入（mock 模式为空字符串）。
// schemaRetries 同理由调用方传入。
func BuildSummary(
	startedAt, endedAt time.Time,
	llmCalls []llm.CallRecord,
	sess *domain.Session,
	schemaRetries int,
) RunSummary {
	s := RunSummary{
		TotalDurationMS:    endedAt.Sub(startedAt).Milliseconds(),
		TotalLLMCalls:      len(llmCalls),
		SchemaRetries:      schemaRetries,
		ErrorBuckets:       map[string]int{},
		ReportOverallScore: -1,
	}
	for _, r := range llmCalls {
		s.TotalPromptTokens += r.PromptTokens
		s.TotalCompletionTokens += r.CompletionTokens
		if r.ErrClass != "ok" && r.ErrClass != "" {
			s.ErrorBuckets[r.ErrClass]++
		}
	}
	if sess != nil {
		s.Rounds = len(sess.Rounds)
		if sess.Report != nil {
			s.ReportOverallScore = sess.Report.OverallScore
		}
	}
	return s
}

func escapeMD(s string) string {
	// 把 | 和换行替换成视觉占位，避免破坏表格
	r := strings.NewReplacer(
		"|", "\\|",
		"\n", " ",
		"\r", " ",
	)
	out := r.Replace(s)
	if len(out) > 80 {
		out = out[:80] + "..."
	}
	return out
}
