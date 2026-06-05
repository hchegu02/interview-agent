package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/agentkit"
	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
	"interview-agent/internal/llm"
)

// evaluate 节点设计要点:
//
//   1. 输入来源 = sess.CurrentRound():
//      pick_next 已经创建了 round 并写了 Question,suspend 等用户。
//      HTTP 层把用户答案填到 round.Answer,然后调 Resume 推到 evaluate。
//      evaluate 不读 PendingAnswer/PendingDecision——这俩字段在 pick_next 阶段就是
//      "出题的意图",答题后由 evaluate 这里消费 + 清空。
//
//   2. 空答案短路:
//      Answer == "" 或纯空白时直接构造 Evaluation{Score:0, Suggestion:"未作答"},
//      不调 LLM——既省 token 又保证语义明确(LLM 自己判断"空"反而可能给随机分)。
//
//   3. ExpectedPoints 喂 LLM:
//      把题库 seed 里的"期望要点"塞进 prompt,让 LLM 对照要点打分,
//      避免单凭印象。要点为空时 prompt 显式标注,LLM 不会因此卡住。
//
//   4. 失败降级:
//      Schema 自校正后仍失败 → 写 Score=-1 + Suggestion="评估失败" 的 degraded eval。
//      为什么 -1 而非 0:critic 节点能据此识别"评估本身失败"区别于"答得差",
//      不会触发无意义的 refine。
//
//   5. 写回时机:
//      LLM 评估完后清空 sess.PendingDecision(它属于"已完成的决策"),
//      把 Evaluation 写到 CurrentRound().Evaluation。
//      CompletedAt 不在这里写——critic/refine/probe 链路可能还要碰这个 round,
//      只有 update_memory 节点最后才标记 CompletedAt。

// EvaluateOptions 暴露给图组装的可调参数。
type EvaluateOptions struct {
	Temperature float64 // 默认 0.2
	MaxTokens   int     // 默认 600
	Hook        agentkit.Hook
}

type evalShape struct {
	QuestionID string   `json:"question_id"`
	Score      int      `json:"score"`
	Strengths  []string `json:"strengths"`
	Weaknesses []string `json:"weaknesses"`
	Suggestion string   `json:"suggestion"`
}

func validateEvaluation(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	if err := llm.ValidateFields(raw, "question_id", "score", "strengths", "weaknesses", "suggestion"); err != nil {
		return err
	}
	var s evalShape
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Score < 0 || s.Score > 100 {
		return fmt.Errorf("score %d not in [0,100]", s.Score)
	}
	return nil
}

// NewEvaluateNode 构造 evaluate 节点。
//
// 节点契约:
//
//	输入:  sess.CurrentRound() 必须存在且 Question 已填; Answer 可为空
//	输出:  CurrentRound().Evaluation 被填; PendingDecision 清空
//	返回:  nil(始终,失败走降级);
//	       ErrPermanent 仅当 CurrentRound() 为 nil(说明上游节点链路 bug)
func NewEvaluateNode(model llm.ChatModel, opts EvaluateOptions) graph.NodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 600
	}
	if opts.Hook == nil {
		opts.Hook = agentkit.NoopHook{}
	}

	return func(ctx context.Context, sess *domain.Session) (err error) {
		start := time.Now()
		_ = opts.Hook.HandleHook(ctx, agentkit.HookEvent{
			Type:         agentkit.HookBeforeSkill,
			SessionID:    sess.ID,
			Name:         "answer.evaluate",
			InputSummary: "current round question and answer",
			Permission:   agentkit.PermissionWriteSession,
		})
		defer func() {
			summary := "evaluation=missing"
			if round := sess.CurrentRound(); round != nil && round.Evaluation != nil {
				summary = fmt.Sprintf("question_id=%s score=%d", round.Evaluation.QuestionID, round.Evaluation.Score)
			}
			ev := agentkit.HookEvent{
				Type:          agentkit.HookAfterSkill,
				SessionID:     sess.ID,
				Name:          "answer.evaluate",
				InputSummary:  "current round question and answer",
				OutputSummary: summary,
				Latency:       time.Since(start),
				Permission:    agentkit.PermissionWriteSession,
			}
			if err != nil {
				ev.Error = err.Error()
			}
			_ = opts.Hook.HandleHook(ctx, ev)
		}()

		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("evaluate: no current round: %w", graph.ErrPermanent)
		}
		if round.Question.ID == "" {
			return fmt.Errorf("evaluate: round has no question: %w", graph.ErrPermanent)
		}

		// 1. 空答案短路
		if strings.TrimSpace(round.Answer) == "" {
			eval := &domain.Evaluation{
				QuestionID: round.Question.ID,
				Score:      0,
				Strengths:  []string{},
				Weaknesses: []string{"候选人未作答"},
				Suggestion: "本题未作答,建议下次至少给出思考方向",
			}
			return applyNodePatch(sess, "evaluate", domain.StatePatch{
				ClearPendingDecision: true,
				CurrentEvaluation:    eval,
			})
		}

		// 2. LLM 评估
		eval, err := evaluateByLLM(ctx, model, round, opts)
		if err != nil {
			// 3. 降级:写一个明显标识"评估失败"的 eval,会话继续
			markEvalFallback(sess, err.Error())
			eval := &domain.Evaluation{
				QuestionID: round.Question.ID,
				Score:      -1,
				Strengths:  []string{},
				Weaknesses: []string{},
				Suggestion: fmt.Sprintf("评估失败(降级): %s", err.Error()),
			}
			return applyNodePatch(sess, "evaluate", domain.StatePatch{
				ClearPendingDecision: true,
				CurrentEvaluation:    eval,
			})
		}

		// 注意:CompletedAt 不在这里写,留给 update_memory 节点统一标记
		return applyNodePatch(sess, "evaluate", domain.StatePatch{
			ClearPendingDecision: true,
			CurrentEvaluation:    eval,
		})
	}
}

func evaluateByLLM(
	ctx context.Context,
	model llm.ChatModel,
	round *domain.AnswerRound,
	opts EvaluateOptions,
) (*domain.Evaluation, error) {
	if model == nil {
		return nil, fmt.Errorf("llm disabled")
	}

	expected := "(无参考要点,按通用标准评估)"
	if pts := round.Question.ExpectedPoints; len(pts) > 0 {
		var sb strings.Builder
		for i, p := range pts {
			fmt.Fprintf(&sb, "  %d. %s\n", i+1, p)
		}
		expected = sb.String()
	}

	prompt := fmt.Sprintf(promptEvaluate,
		round.Question.Content,
		expected,
		round.Answer,
		round.Question.ID,
	)
	messages := []llm.Message{{Role: "system", Content: prompt}}
	llmOpts := llm.Options{Temperature: opts.Temperature, MaxTokens: opts.MaxTokens}

	resp, err := llm.CallWithSchema(ctx, model, messages, llmOpts, validateEvaluation, 1)
	if err != nil {
		return nil, err
	}
	var s evalShape
	if err := json.Unmarshal([]byte(resp.Content), &s); err != nil {
		return nil, fmt.Errorf("unmarshal eval: %w", err)
	}
	// LLM 可能填错 question_id,以 round 里的为准
	return &domain.Evaluation{
		QuestionID: round.Question.ID,
		Score:      s.Score,
		Strengths:  s.Strengths,
		Weaknesses: s.Weaknesses,
		Suggestion: strings.TrimSpace(s.Suggestion),
	}, nil
}

func markEvalFallback(sess *domain.Session, reason string) {
	if sess.WorkingMemory == nil {
		sess.WorkingMemory = domain.NewWorkingMemory()
	}
	markDegradedReason(sess.WorkingMemory, "eval", reason)
}
