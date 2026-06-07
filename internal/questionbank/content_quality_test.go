package questionbank

import "testing"

func TestQuestionContentQualityFlagsDirtyInterviewNoteChain(t *testing.T) {
	content := "有使用过吗你这个agent项目就是四个智能体 用langchain也可以实现啊 你用了langGraph 有哪些是用langchain不能实现的吗--（无法反驳.."

	result := EvaluateQuestionContentQuality(content)

	if !result.HighRisk {
		t.Fatalf("HighRisk = false, want true: %+v", result)
	}
	for _, want := range []string{QualityFlagDirtyNoteMarker, QualityFlagMultipleQuestionChain, QualityFlagAnswerOrCommentLeak} {
		if !contains(result.Flags, want) {
			t.Fatalf("flags = %v, want %s", result.Flags, want)
		}
	}
}

func TestQuestionContentQualityAllowsNormalQuestion(t *testing.T) {
	content := "你的 Agent 项目为什么选择 Graph 流程编排，而不是直接使用 LangChain？请从状态管理、可恢复性和可观测性角度说明。"

	result := EvaluateQuestionContentQuality(content)

	if result.HighRisk || len(result.Flags) != 0 {
		t.Fatalf("quality = %+v, want clean", result)
	}
}

func TestQuestionContentQualityTreatsNormalThreePartQuestionAsAdvisory(t *testing.T) {
	content := "CAP 是什么？为什么不能三者兼得？项目里怎么取舍？"

	result := EvaluateQuestionContentQuality(content)

	if result.HighRisk {
		t.Fatalf("HighRisk = true, want advisory only: %+v", result)
	}
	if !contains(result.AdvisoryFlags, QualityFlagMultipleQuestionChain) {
		t.Fatalf("AdvisoryFlags = %v, want %s", result.AdvisoryFlags, QualityFlagMultipleQuestionChain)
	}
}
