package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
	"interview-agent/internal/retriever"
)

// jdSchemaShape 是 parse_jd 节点期待的 LLM 输出 JSON 形状。
// 单独定义而不复用 domain.JobProfile，避免把 JDRawText 这种"运行时元数据"
// 字段塞进 LLM schema——LLM 不该看到、也不会输出原文。
type jdSchemaShape struct {
	Title         string   `json:"title"`
	Level         string   `json:"level"`
	KeySkills     []string `json:"key_skills"`
	MustHave      []string `json:"must_have"`
	NiceToHave    []string `json:"nice_to_have"`
	YearsRequired int      `json:"years_required"`
}

// validJDLevels 是 level 字段允许的枚举值。
var validJDLevels = []string{"intern", "junior", "senior", "staff"}

// validateJDSchema 是 parse_jd 的 schema validator。
//
// 校验链：
//  1. 合法 JSON
//  2. 必填字段存在
//  3. level 枚举值合法
//  4. title 非空
//  5. years_required >= 0
//
// 任何一条失败都会被回灌给 LLM 自纠正。
func validateJDSchema(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw,
		"title", "level", "key_skills", "must_have", "nice_to_have", "years_required"); err != nil {
		return err
	}
	if err := llm.ValidateEnum(raw, "level", validJDLevels...); err != nil {
		return err
	}
	var shape jdSchemaShape
	if err := json.Unmarshal(raw, &shape); err != nil {
		return fmt.Errorf("parse jd shape: %w", err)
	}
	if strings.TrimSpace(shape.Title) == "" {
		return errors.New("title is required")
	}
	if shape.YearsRequired < 0 {
		return fmt.Errorf("years_required must be >= 0, got %d", shape.YearsRequired)
	}
	return nil
}

// NewParseJDNode 构造 parse_jd 节点。
//
// 节点契约：
//   输入：sess.JobProfile.JDRawText（必须非空，由 setup 阶段前置填入）
//   输出：sess.JobProfile.{Title, Level, KeySkills, MustHave, NiceToHave, YearsRequired}
//
// 失败语义：
//   - JD 为空 → graph.ErrPermanent（不可重试，配置错误）
//   - LLM 调用失败 → 原 error 冒泡（Retry 装饰器决定是否重试）
//   - schema 多次校验失败 → ErrSchemaInvalid（外层 Retry 可包成 ErrPermanent）
//
// 关于 tags 归一化：
//   LLM 已经做了一轮"Golang→go"的合并，但保险起见再过一遍 retriever.CanonicalizeTags，
//   保证最终入 KeySkills 的全是 canonical 形式，与题库 tags 直接可比对。
func NewParseJDNode(model llm.ChatModel) graph.NodeFunc {
	return func(ctx context.Context, sess *domain.Session) error {
		if sess.JobProfile == nil || strings.TrimSpace(sess.JobProfile.JDRawText) == "" {
			return fmt.Errorf("parse_jd: jd raw text is required: %w", graph.ErrPermanent)
		}

		messages := []llm.Message{
			{Role: "system", Content: fmt.Sprintf(promptParseJD, sess.JobProfile.JDRawText)},
		}
		opts := llm.Options{
			Temperature: 0.1, // schema 抽取任务用低温度，减少漂移
			MaxTokens:   800,
		}

		resp, err := llm.CallWithSchema(ctx, model, messages, opts, validateJDSchema, 1)
		if err != nil {
			return fmt.Errorf("parse_jd: %w", err)
		}

		var shape jdSchemaShape
		if err := json.Unmarshal([]byte(resp.Content), &shape); err != nil {
			// 已经过 schema validator，这里不该失败；防御性兜底
			return fmt.Errorf("parse_jd: unmarshal after validate: %w", err)
		}

		// 归一化 tags
		shape.KeySkills = retriever.CanonicalizeTags(shape.KeySkills)
		shape.MustHave = retriever.CanonicalizeTags(shape.MustHave)
		shape.NiceToHave = retriever.CanonicalizeTags(shape.NiceToHave)

		sess.JobProfile.Title = shape.Title
		sess.JobProfile.Level = shape.Level
		sess.JobProfile.KeySkills = shape.KeySkills
		sess.JobProfile.MustHave = shape.MustHave
		sess.JobProfile.NiceToHave = shape.NiceToHave
		sess.JobProfile.YearsRequired = shape.YearsRequired
		return nil
	}
}
