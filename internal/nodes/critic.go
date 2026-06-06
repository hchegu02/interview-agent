package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// critic 节点设计要点:
//
//   1. 双信号合并:
//      "评估反思"和"探问信号"两个判断的上下文几乎完全重叠
//      (question + answer + evaluation + expected_points),
//      拆成两个节点会让 LLM 在两次调用里读同样的内容、推同样的事——浪费。
//      改为一次 LLM 调用同时输出 5 个字段:grounded_score / need_refine / issues / summary
//      + has_probe_signal / probe_topic。
//
//   2. 预算前置:
//      WorkingMemory.CanProbe() 在节点内检查;预算耗尽时即使 LLM 说 has_probe_signal=true,
//      也强制改成 false。这样下游 router 看不到信号 → 不走 probe 分支,
//      节点之间不需要再传"预算还有没"。
//
//   3. 失败短路:
//      Evaluation 不存在或 Score==-1 (评估降级) 时,critic 直接放过——
//      给一个 NeedRefine=false / HasProbeSignal=false 的"放行 Critic",
//      不烧 token,避免在已经降级的评估上继续追加 LLM 调用。
//
//   4. 失败降级:
//      LLM 出错时同样写"放行 Critic",会话继续 + DegradedReasons 打 critic。
//      Critic 是流程兜底,本身降级不能让会话挂掉。

// CriticOptions 暴露给图组装的可调参数。
type CriticOptions struct {
	Temperature float64 // 默认 0.2
	MaxTokens   int     // 默认 500
	// GroundedScore 低于此值触发 refine,默认 60
	RefineThreshold int
}

type criticShape struct {
	GroundedScore  int      `json:"grounded_score"`
	NeedRefine     bool     `json:"need_refine"`
	Issues         []string `json:"issues"`
	Summary        string   `json:"summary"`
	HasProbeSignal bool     `json:"has_probe_signal"`
	ProbeTopic     string   `json:"probe_topic"`
}

func validateCritic(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw,
		"grounded_score", "need_refine", "issues", "summary",
		"has_probe_signal", "probe_topic"); err != nil {
		return err
	}
	var s criticShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.GroundedScore < 0 || s.GroundedScore > 100 {
		return fmt.Errorf("grounded_score %d not in [0,100]", s.GroundedScore)
	}
	return nil
}

// NewCriticNode 构造 critic 节点。
//
// 节点契约:
//
//	输入: CurrentRound() 必须存在,Evaluation 必须已填(可为降级 score=-1)
//	输出: round.CriticResult 被填(始终,失败走"放行")
//	返回: nil(始终);ErrPermanent 仅当 CurrentRound() / Evaluation 为 nil
func NewCriticNode(model llm.ChatModel, opts CriticOptions) graph.NodeFunc {
	patchNode := NewCriticPatchNode(model, opts)
	return func(ctx context.Context, sess *domain.Session) error {
		patch, err := patchNode(ctx, sess)
		if err != nil {
			return err
		}
		return applyNodePatch(sess, "critic", patch)
	}
}

// NewCriticPatchNode 构造由 Graph runner 统一应用 StatePatch 的 critic 节点。
func NewCriticPatchNode(model llm.ChatModel, opts CriticOptions) graph.PatchNodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 500
	}
	if opts.RefineThreshold == 0 {
		opts.RefineThreshold = 60
	}

	return func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
		round := sess.CurrentRound()
		if round == nil {
			return domain.StatePatch{}, fmt.Errorf("critic: no current round: %w", graph.ErrPermanent)
		}
		if round.Evaluation == nil {
			return domain.StatePatch{}, fmt.Errorf("critic: evaluation required: %w", graph.ErrPermanent)
		}
		mem := cloneWorkingMemory(sess.WorkingMemory)

		// 1. 评估降级 → 直接放行 critic,不烧 token
		if round.Evaluation.Score < 0 {
			patch := domain.StatePatch{CurrentCriticResult: &domain.Critic{
				GroundedScore:  -1,
				NeedRefine:     false,
				Summary:        "上游评估已降级,跳过 critic",
				HasProbeSignal: false,
			}}
			if sess.WorkingMemory == nil {
				patch.WorkingMemory = mem
			}
			return patch, nil
		}

		// 2. LLM 调用
		shape, err := criticByLLM(ctx, model, round, opts)
		if err != nil {
			markDegradedReason(mem, "critic", err.Error())
			return domain.StatePatch{
				CurrentCriticResult: &domain.Critic{
					GroundedScore:  -1,
					NeedRefine:     false,
					Summary:        fmt.Sprintf("critic 降级: %s", err.Error()),
					HasProbeSignal: false,
				},
				WorkingMemory: mem,
			}, nil
		}

		// 3. 预算约束:probe 预算耗尽 → 强制 has_probe_signal=false
		hasProbe := shape.HasProbeSignal
		probeTopic := strings.TrimSpace(shape.ProbeTopic)
		if hasProbe && !mem.CanProbe() {
			hasProbe = false
			probeTopic = ""
		}
		if !hasProbe {
			probeTopic = "" // 信号关闭时清空 topic,避免下游误用
		}

		// 4. 决定 need_refine:LLM 说要 refine 或 grounded_score 低于阈值都触发
		needRefine := shape.NeedRefine || shape.GroundedScore < opts.RefineThreshold

		patch := domain.StatePatch{CurrentCriticResult: &domain.Critic{
			GroundedScore:  shape.GroundedScore,
			NeedRefine:     needRefine,
			Issues:         shape.Issues,
			Summary:        strings.TrimSpace(shape.Summary),
			HasProbeSignal: hasProbe,
			ProbeTopic:     probeTopic,
		}}
		if sess.WorkingMemory == nil {
			patch.WorkingMemory = mem
		}
		return patch, nil
	}
}

func criticByLLM(
	ctx context.Context,
	model llm.ChatModel,
	round *domain.AnswerRound,
	opts CriticOptions,
) (*criticShape, error) {
	if model == nil {
		return nil, fmt.Errorf("llm disabled")
	}

	expected := "(无)"
	if pts := round.Question.ExpectedPoints; len(pts) > 0 {
		var sb strings.Builder
		for i, p := range pts {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, p)
		}
		expected = sb.String()
	}

	prompt := fmt.Sprintf(promptCritic,
		round.Question.Content,
		round.Answer,
		round.Evaluation.Score,
		round.Evaluation.Strengths,
		round.Evaluation.Weaknesses,
		round.Evaluation.Suggestion,
		expected,
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validateCritic, 1)
	if err != nil {
		return nil, err
	}
	var shape criticShape
	if err := json.Unmarshal([]byte(resp.Content), &shape); err != nil {
		return nil, fmt.Errorf("unmarshal critic: %w", err)
	}
	return &shape, nil
}
