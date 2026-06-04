package agentkit

import "testing"

func TestDefaultInterviewSkills(t *testing.T) {
	reg := NewSkillRegistry()
	if err := RegisterDefaultInterviewSkills(reg); err != nil {
		t.Fatalf("register defaults: %v", err)
	}
	want := []string{
		"answer.evaluate",
		"jd.analyze",
		"profile.match",
		"question.retrieve",
		"report.generate",
		"resume.parse",
	}
	got := reg.List()
	if len(got) != len(want) {
		t.Fatalf("skill count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("skill[%d] = %s, want %s", i, got[i].Name, name)
		}
		if got[i].Version != "v1" || got[i].Description == "" {
			t.Fatalf("skill[%d] incomplete: %+v", i, got[i])
		}
	}
}
