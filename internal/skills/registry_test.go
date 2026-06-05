package skills

import (
	"context"
	"errors"
	"testing"
)

func TestRegistry_RunsRegisteredSkill(t *testing.T) {
	reg := NewDefaultRegistry()

	result, err := reg.Run(context.Background(), "quiz", SkillInput{Message: "给我出一道 Go 题"})
	if err != nil {
		t.Fatalf("run skill: %v", err)
	}
	if result.Title == "" || result.Content == "" {
		t.Fatalf("result should be populated: %+v", result)
	}
}

func TestRegistry_UnknownSkill(t *testing.T) {
	reg := NewDefaultRegistry()

	_, err := reg.Run(context.Background(), "missing", SkillInput{Message: "x"})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("error = %v, want ErrSkillNotFound", err)
	}
}
