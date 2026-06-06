package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// probe 链路设计要点:
//
//   1. 两节点 probe_ask + probe_eval, 节点拆分对齐 pick_next + evaluate:
//      probe_ask 生成追问问题 + suspend 等用户答; probe_eval 评分追答 + 决定是否再追。
//      与"主题题"路径(pick_next→evaluate)结构对称, suspend/resume 的 HTTP 协议复用。
//
//   2. 多轮 probe 通过"重写 CriticResult 信号"实现循环边:
//      probe_eval 双输出: 既给追答打分 + 决定是否继续追问。
//      它把新的 HasProbeSignal/ProbeTopic 写回 round.CriticResult,
//      下游 router 看 CriticResult.HasProbeSignal 决定走 probe_ask 还是 update_memory。
//      连环追问的预算靠 WorkingMemory.MaxProbes 兜底, probe_ask 自增 ProbesUsed。
//
//   3. 共享 round.FollowUps slice:
//      每轮追问追加一个 FollowUp{Question, Answer:"", Reason}, suspend 等用户;
//      Resume 后 HTTP 层填 FollowUps[last].Answer, probe_eval 读 last 项评分。
//      列表天然按时间排, report 能直接顺序回放。
//
//   4. 失败兜底:
//      probe_ask 失败 → 跳过这次追问, 把 CriticResult.HasProbeSignal=false 让 router 走 update_memory
//      probe_eval 失败 → 给追答写 score=-1 的 degraded eval, 同样关掉再追信号
//      节点本身不返回 error(除了 ErrPermanent 的脏数据 case)

// =============================================================================
// probe_ask
// =============================================================================

type ProbeAskOptions struct {
	Temperature float64 // 默认 0.3, 追问要有一点发散
	MaxTokens   int     // 默认 200
}

type probeAskShape struct {
	Question string `json:"question"`
	Reason   string `json:"reason"`
}

func validateProbeAsk(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw, "question", "reason"); err != nil {
		return err
	}
	var s probeAskShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if strings.TrimSpace(s.Question) == "" {
		return fmt.Errorf("question is empty")
	}
	return nil
}

// NewProbeAskNode 构造 probe_ask 节点。
//
// 节点契约:
//
//	输入: CurrentRound() 存在, CriticResult.HasProbeSignal==true, CriticResult.ProbeTopic 非空
//	输出: round.FollowUps 追加一项, ProbesUsed++
//	返回: ErrSuspended (成功) | nil (降级: 关掉 probe 信号让 router 跳过)
//	      ErrPermanent: round/critic 缺
func NewProbeAskNode(model llm.ChatModel, opts ProbeAskOptions) graph.NodeFunc {
	patchNode := NewProbeAskPatchNode(model, opts)
	return func(ctx context.Context, sess *domain.Session) error {
		patch, err := patchNode(ctx, sess)
		if err != nil {
			if graph.IsPatchSuspend(err) {
				if applyErr := applyNodePatch(sess, "probe_ask", patch); applyErr != nil {
					return applyErr
				}
			}
			return err
		}
		return applyNodePatch(sess, "probe_ask", patch)
	}
}

// NewProbeAskPatchNode 构造由 Graph runner 统一应用 StatePatch 的 probe_ask 节点。
func NewProbeAskPatchNode(model llm.ChatModel, opts ProbeAskOptions) graph.PatchNodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.3
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 200
	}

	return func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
		round := sess.CurrentRound()
		if round == nil {
			return domain.StatePatch{}, fmt.Errorf("probe_ask: no current round: %w", graph.ErrPermanent)
		}
		if round.CriticResult == nil {
			return domain.StatePatch{}, fmt.Errorf("probe_ask: critic required: %w", graph.ErrPermanent)
		}
		mem := cloneWorkingMemory(sess.WorkingMemory)

		// 节点自检(router 通常已挡掉)
		if !round.CriticResult.HasProbeSignal || !mem.CanProbe() {
			if sess.WorkingMemory == nil {
				return domain.StatePatch{WorkingMemory: mem}, nil
			}
			return domain.StatePatch{}, nil
		}

		question, reason, err := probeAskByLLM(ctx, model, round, opts)
		if err != nil {
			// 降级: 关掉信号让下游 router 走 update_memory, 会话继续
			markDegradedReason(mem, "probe_ask", err.Error())
			return domain.StatePatch{
				CurrentCriticProbeSignal: &domain.CriticProbeSignalPatch{
					HasProbeSignal: false,
					ProbeTopic:     "",
				},
				WorkingMemory: mem,
			}, nil
		}

		followUp := &domain.FollowUp{
			Question: question,
			Reason:   reason,
			AskedAt:  time.Now(),
		}
		mem.ProbesUsed++
		patch := domain.StatePatch{
			AppendCurrentFollowUp: followUp,
			WorkingMemory:         mem,
		}

		return patch, graph.SuspendWithPatch(fmt.Errorf("probe_ask: waiting for follow-up answer (probes_used=%d): %w",
			mem.ProbesUsed, graph.ErrSuspended))
	}
}

func probeAskByLLM(
	ctx context.Context,
	model llm.ChatModel,
	round *domain.AnswerRound,
	opts ProbeAskOptions,
) (string, string, error) {
	if model == nil {
		return "", "", fmt.Errorf("llm disabled")
	}

	// 之前的追问历史(多轮 probe 时让 LLM 不重复问)
	priorFollowUps := "(无)"
	if len(round.FollowUps) > 0 {
		var sb strings.Builder
		sb.WriteString("此前已问过的追问:\n")
		for i, f := range round.FollowUps {
			fmt.Fprintf(&sb, "  追问%d: %s\n  回答: %s\n", i+1, f.Question, truncate(f.Answer, 60))
		}
		priorFollowUps = sb.String()
	}

	prompt := fmt.Sprintf(promptProbeAsk,
		round.Question.Content,
		round.Answer,
		priorFollowUps,
		round.CriticResult.ProbeTopic,
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validateProbeAsk, 1)
	if err != nil {
		return "", "", err
	}
	var s probeAskShape
	if err := json.Unmarshal([]byte(resp.Content), &s); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(s.Question), strings.TrimSpace(s.Reason), nil
}

// =============================================================================
// probe_eval
// =============================================================================

type ProbeEvalOptions struct {
	Temperature float64 // 默认 0.2
	MaxTokens   int     // 默认 500
}

type probeEvalShape struct {
	Score          int      `json:"score"`
	Strengths      []string `json:"strengths"`
	Weaknesses     []string `json:"weaknesses"`
	Suggestion     string   `json:"suggestion"`
	HasMoreProbe   bool     `json:"has_more_probe"`
	NextProbeTopic string   `json:"next_probe_topic"`
}

func validateProbeEval(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw,
		"score", "strengths", "weaknesses", "suggestion",
		"has_more_probe", "next_probe_topic"); err != nil {
		return err
	}
	var s probeEvalShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Score < 0 || s.Score > 100 {
		return fmt.Errorf("score %d not in [0,100]", s.Score)
	}
	return nil
}

// NewProbeEvalNode 构造 probe_eval 节点。
//
// 节点契约:
//
//	输入: CurrentRound() 存在, FollowUps 非空, last FollowUp.Answer 已填(可空字符串)
//	输出: last FollowUp.Evaluation 填入; round.CriticResult.HasProbeSignal/ProbeTopic
//	      按 LLM 决策 + 预算约束更新
//	返回: nil (始终, 失败走降级); ErrPermanent: round/followup 缺
func NewProbeEvalNode(model llm.ChatModel, opts ProbeEvalOptions) graph.NodeFunc {
	patchNode := NewProbeEvalPatchNode(model, opts)
	return func(ctx context.Context, sess *domain.Session) error {
		patch, err := patchNode(ctx, sess)
		if err != nil {
			return err
		}
		return applyNodePatch(sess, "probe_eval", patch)
	}
}

// NewProbeEvalPatchNode 构造由 Graph runner 统一应用 StatePatch 的 probe_eval 节点。
func NewProbeEvalPatchNode(model llm.ChatModel, opts ProbeEvalOptions) graph.PatchNodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 500
	}

	return func(ctx context.Context, sess *domain.Session) (domain.StatePatch, error) {
		round := sess.CurrentRound()
		if round == nil {
			return domain.StatePatch{}, fmt.Errorf("probe_eval: no current round: %w", graph.ErrPermanent)
		}
		if len(round.FollowUps) == 0 {
			return domain.StatePatch{}, fmt.Errorf("probe_eval: no follow-up: %w", graph.ErrPermanent)
		}
		last := &round.FollowUps[len(round.FollowUps)-1]
		mem := cloneWorkingMemory(sess.WorkingMemory)

		// 空追答短路: 与 evaluate 对称
		if strings.TrimSpace(last.Answer) == "" {
			eval := &domain.Evaluation{
				QuestionID: round.Question.ID + "-followup",
				Score:      0,
				Strengths:  []string{},
				Weaknesses: []string{"候选人对追问未作答"},
				Suggestion: "追问未作答, 跳过深挖",
			}
			patch := domain.StatePatch{CurrentFollowUpEvaluation: eval}
			if round.CriticResult != nil {
				patch.CurrentCriticProbeSignal = &domain.CriticProbeSignalPatch{HasProbeSignal: false, ProbeTopic: ""}
			}
			if sess.WorkingMemory == nil {
				patch.WorkingMemory = mem
			}
			return patch, nil
		}

		shape, err := probeEvalByLLM(ctx, model, round, last, opts)
		if err != nil {
			markDegradedReason(mem, "probe_eval", err.Error())
			eval := &domain.Evaluation{
				QuestionID: round.Question.ID + "-followup",
				Score:      -1,
				Suggestion: fmt.Sprintf("追答评估失败(降级): %s", err.Error()),
			}
			patch := domain.StatePatch{
				CurrentFollowUpEvaluation: eval,
				WorkingMemory:             mem,
			}
			if round.CriticResult != nil {
				patch.CurrentCriticProbeSignal = &domain.CriticProbeSignalPatch{HasProbeSignal: false, ProbeTopic: ""}
			}
			return patch, nil
		}

		eval := &domain.Evaluation{
			QuestionID: round.Question.ID + "-followup",
			Score:      shape.Score,
			Strengths:  shape.Strengths,
			Weaknesses: shape.Weaknesses,
			Suggestion: strings.TrimSpace(shape.Suggestion),
		}
		patch := domain.StatePatch{CurrentFollowUpEvaluation: eval}

		// 更新 critic 信号供 router 做"是否再追"判断
		if round.CriticResult != nil {
			hasMore := shape.HasMoreProbe
			topic := strings.TrimSpace(shape.NextProbeTopic)
			if hasMore && !mem.CanProbe() {
				hasMore = false
			}
			if !hasMore {
				topic = ""
			}
			patch.CurrentCriticProbeSignal = &domain.CriticProbeSignalPatch{
				HasProbeSignal: hasMore,
				ProbeTopic:     topic,
			}
		}
		if sess.WorkingMemory == nil {
			patch.WorkingMemory = mem
		}
		return patch, nil
	}
}

func probeEvalByLLM(
	ctx context.Context,
	model llm.ChatModel,
	round *domain.AnswerRound,
	last *domain.FollowUp,
	opts ProbeEvalOptions,
) (*probeEvalShape, error) {
	if model == nil {
		return nil, fmt.Errorf("llm disabled")
	}

	prompt := fmt.Sprintf(promptProbeEval,
		round.Question.Content,
		last.Question,
		last.Answer,
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validateProbeEval, 1)
	if err != nil {
		return nil, err
	}
	var s probeEvalShape
	if err := json.Unmarshal([]byte(resp.Content), &s); err != nil {
		return nil, fmt.Errorf("unmarshal probe_eval: %w", err)
	}
	return &s, nil
}
