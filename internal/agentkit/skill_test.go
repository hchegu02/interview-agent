package agentkit

import "testing"

func TestSkillRegistryRegistersAndListsStable(t *testing.T) {
	reg := NewSkillRegistry()
	err := reg.Register(SkillSpec{Name: "resume.parse", Version: "v1", Description: "parse resume", Permission: PermissionReadOnly})
	if err != nil {
		t.Fatalf("register resume.parse: %v", err)
	}
	err = reg.Register(SkillSpec{Name: "jd.analyze", Version: "v1", Description: "analyze jd", Permission: PermissionReadOnly})
	if err != nil {
		t.Fatalf("register jd.analyze: %v", err)
	}

	spec, ok := reg.Get("resume.parse")
	if !ok {
		t.Fatal("resume.parse not found")
	}
	if spec.Name != "resume.parse" || spec.Permission != PermissionReadOnly {
		t.Fatalf("spec = %+v", spec)
	}

	list := reg.List()
	if len(list) != 2 || list[0].Name != "jd.analyze" || list[1].Name != "resume.parse" {
		t.Fatalf("list not stable sorted: %+v", list)
	}
}

func TestSkillRegistryRejectsDuplicateAndInvalidName(t *testing.T) {
	reg := NewSkillRegistry()
	if err := reg.Register(SkillSpec{Name: "", Version: "v1"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := reg.Register(SkillSpec{Name: "answer.evaluate", Version: "v1"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.Register(SkillSpec{Name: "answer.evaluate", Version: "v2"}); err == nil {
		t.Fatal("duplicate register should fail")
	}
}
