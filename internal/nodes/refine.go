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

// refine 节点设计要点:
//
//   1. 复用 evaluate 的 schema:
//      refine 的输出就是"修正后的 Evaluation",所以 schema 完全一致——
//      validator 直接复用 validateEvaluation。这样上游 critic 看 score 字段、
//      下游 update_memory 取均分都不需要分两种 eval 类型处理。
//
//   2. 写 RefinedEval 而非覆盖 Evaluation:
//      AnswerRound.Evaluation 保留原评估(审计),RefinedEval 单独存修正后的。
//      Round.FinalEvaluation() 自动返回 RefinedEval > Evaluation,
//      report 节点统计成绩用这个,业务无需感知是否被 refine。
//
//   3. 前置校验:
//      只在 CriticResult.NeedRefine == true 时执行——理论上 router 已经过滤,
//      但节点自检一遍防止误调用。
//
//   4. 失败兜底:
//      refine 失败 → RefinedEval=nil(不破坏原 Evaluation),DegradedReasons 打 refine。
//      report 拿到的还是原评估,会话不中断。

type RefineOptions struct {
	Temperature float64 // 默认 0.2,refine 需要稳一点
	MaxTokens   int     // 默认 600
}

// NewRefineNode 构造 refine 节点。
//
// 节点契约:
//   输入: CurrentRound() 必须存在,Evaluation 和 CriticResult 都已填
//   输出: round.RefinedEval(成功) 或保持 nil(失败,但 round 不破坏)
//   返回: nil(始终);ErrPermanent 仅当 round / eval / critic 为 nil
func NewRefineNode(model llm.ChatModel, opts RefineOptions) graph.NodeFunc {
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 600
	}

	return func(ctx context.Context, sess *domain.Session) error {
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("refine: no current round: %w", graph.ErrPermanent)
		}
		if round.Evaluation == nil || round.CriticResult == nil {
			return fmt.Errorf("refine: requires eval+critic: %w", graph.ErrPermanent)
		}
		// 节点自检:critic 说不需要 refine 就直接放过(router 通常已挡掉)
		if !round.CriticResult.NeedRefine {
			return nil
		}

		refined, err := refineByLLM(ctx, model, round, opts)
		if err != nil {
			markRefineFallback(sess, err.Error())
			// 不覆盖原 evaluation,RefinedEval 保持 nil,
			// FinalEvaluation() 会回退到原 evaluation
			return nil
		}
		round.RefinedEval = refined
		return nil
	}
}

func refineByLLM(
	ctx context.Context,
	model llm.ChatModel,
	round *domain.AnswerRound,
	opts RefineOptions,
) (*domain.Evaluation, error) {
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

	issuesStr := strings.Join(round.CriticResult.Issues, "; ")
	if issuesStr == "" {
		issuesStr = "(critic 未给出具体 issues)"
	}

	prompt := fmt.Sprintf(promptRefine,
		round.Question.Content,
		round.Answer,
		expected,
		round.Evaluation.Score,
		round.Evaluation.Strengths,
		round.Evaluation.Weaknesses,
		round.Evaluation.Suggestion,
		issuesStr,
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
		return nil, fmt.Errorf("unmarshal refine: %w", err)
	}
	return &domain.Evaluation{
		QuestionID: round.Question.ID,
		Score:      s.Score,
		Strengths:  s.Strengths,
		Weaknesses: s.Weaknesses,
		Suggestion: strings.TrimSpace(s.Suggestion),
	}, nil
}

func markRefineFallback(sess *domain.Session, reason string) {
	if sess.WorkingMemory == nil {
		sess.WorkingMemory = domain.NewWorkingMemory()
	}
	markDegradedReason(sess.WorkingMemory, "refine", reason)
}
