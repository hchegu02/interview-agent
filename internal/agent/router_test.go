package agent

import "testing"

func TestRuleRouter_RoutesSkillIntents(t *testing.T) {
	router := NewRuleRouter()

	tests := []struct {
		name   string
		text   string
		intent string
		skill  string
	}{
		{name: "quiz", text: "给我出一道 Redis 练习题", intent: IntentSkillQuiz, skill: "quiz"},
		{name: "explain", text: "解释一下 MVCC 原理", intent: IntentSkillExplain, skill: "explain"},
		{name: "project", text: "帮我润色简历项目亮点", intent: IntentSkillProjectPolish, skill: "project_polish"},
		{name: "interview", text: "开始模拟面试", intent: IntentInterviewStart, skill: ""},
		{name: "chat", text: "你好", intent: IntentChat, skill: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.Route(AgentMessage{Message: tt.text})
			if got.Intent != tt.intent || got.Skill != tt.skill {
				t.Fatalf("route = %+v, want intent=%s skill=%s", got, tt.intent, tt.skill)
			}
			if got.Confidence <= 0 {
				t.Fatalf("confidence should be positive: %+v", got)
			}
			if got.Reason == "" {
				t.Fatalf("reason should be set: %+v", got)
			}
		})
	}
}
