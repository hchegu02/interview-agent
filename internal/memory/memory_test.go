package memory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"interview-agent/internal/domain"
)

func TestMemoryStore_SaveReadAndNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if _, err := store.GetUserMemory(ctx, "u1"); !errors.Is(err, ErrUserMemoryNotFound) {
		t.Fatalf("error = %v, want ErrUserMemoryNotFound", err)
	}

	mem := &UserMemory{
		UserID:      "u1",
		Strengths:   []string{"项目表达清楚"},
		Weaknesses:  []Weakness{{Topic: "redis", Evidence: "缓存击穿回答不完整", Severity: 2}},
		SkillScores: map[string]float64{"redis": 62},
		LastAdvice:  []string{"复习 Redis 缓存异常场景"},
		UpdatedAt:   time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertUserMemory(ctx, mem); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	got, err := store.GetUserMemory(ctx, "u1")
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.UserID != "u1" || got.SkillScores["redis"] != 62 {
		t.Fatalf("memory = %+v", got)
	}

	got.SkillScores["redis"] = 100
	again, err := store.GetUserMemory(ctx, "u1")
	if err != nil {
		t.Fatalf("get memory again: %v", err)
	}
	if again.SkillScores["redis"] != 62 {
		t.Fatalf("store leaked internal map, got score %v", again.SkillScores["redis"])
	}
}

func TestMemoryStore_NormalizesUserID(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.UpsertUserMemory(ctx, &UserMemory{
		UserID:      " u1 ",
		SkillScores: map[string]float64{"go": 80},
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	got, err := store.GetUserMemory(ctx, "u1")
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.UserID != "u1" || got.SkillScores["go"] != 80 {
		t.Fatalf("memory = %+v, want normalized user id", got)
	}
}

func TestBuildUpdateFromSession_ExtractsReportSignals(t *testing.T) {
	now := time.Date(2026, 6, 6, 11, 0, 0, 0, time.UTC)
	sess := &domain.Session{
		ID:     "s1",
		UserID: "u1",
		Report: &domain.Report{
			Highlights:     []string{"能结合项目解释限流"},
			Improvements:   []string{"Redis 缓存击穿回答缺少互斥锁方案"},
			NextSteps:      []string{"补充 Redis 缓存异常复习"},
			SkillBreakdown: map[string]int{"go": 82, "redis": 46},
			DrillPlan: []domain.DrillPlanItem{{
				Skill:  "redis",
				Reason: "低分技能需要补练",
			}},
		},
	}

	update, err := BuildUpdateFromSession(sess, now)
	if err != nil {
		t.Fatalf("build update: %v", err)
	}
	if update.UserID != "u1" || update.UpdatedAt != now {
		t.Fatalf("update identity = %+v", update)
	}
	if update.SkillScores["go"] != 82 || update.SkillScores["redis"] != 46 {
		t.Fatalf("skill scores = %+v", update.SkillScores)
	}
	if len(update.Strengths) != 1 || update.Strengths[0] != "能结合项目解释限流" {
		t.Fatalf("strengths = %+v", update.Strengths)
	}
	if len(update.Weaknesses) < 2 {
		t.Fatalf("weaknesses = %+v, want improvement and low skill weakness", update.Weaknesses)
	}
	if len(update.LastAdvice) < 2 {
		t.Fatalf("last advice = %+v, want next steps and drill advice", update.LastAdvice)
	}
}

func TestBuildUpdateFromSession_RequiresUserAndReport(t *testing.T) {
	now := time.Date(2026, 6, 6, 11, 0, 0, 0, time.UTC)

	if _, err := BuildUpdateFromSession(nil, now); err == nil {
		t.Fatal("nil session should fail")
	}
	if _, err := BuildUpdateFromSession(&domain.Session{UserID: "u1"}, now); err == nil {
		t.Fatal("missing report should fail")
	}
	if _, err := BuildUpdateFromSession(&domain.Session{Report: &domain.Report{}}, now); err == nil {
		t.Fatal("missing user id should fail")
	}
}

func TestApplyUpdate_MergesDeterministicallyWithoutMutatingSession(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	existing := &UserMemory{
		UserID:      "u1",
		Strengths:   []string{"基础扎实"},
		Weaknesses:  []Weakness{{Topic: "redis", Evidence: "旧证据", Severity: 1}},
		SkillScores: map[string]float64{"redis": 60, "mysql": 75},
		LastAdvice:  []string{"旧建议"},
	}
	update := UserMemoryUpdate{
		UserID:      "u1",
		Strengths:   []string{"基础扎实", "项目表达清楚"},
		Weaknesses:  []Weakness{{Topic: "redis", Evidence: "新证据", Severity: 3}},
		SkillScores: map[string]float64{"redis": 40, "go": 88},
		LastAdvice:  []string{"旧建议", "补 Redis"},
		UpdatedAt:   now,
	}

	got, err := ApplyUpdate(existing, update)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if got.UserID != "u1" || got.UpdatedAt != now {
		t.Fatalf("identity = %+v", got)
	}
	if len(got.Strengths) != 2 {
		t.Fatalf("strengths = %+v, want deduped merge", got.Strengths)
	}
	if got.SkillScores["redis"] != 50 || got.SkillScores["mysql"] != 75 || got.SkillScores["go"] != 88 {
		t.Fatalf("skill scores = %+v", got.SkillScores)
	}
	if len(got.Weaknesses) != 2 {
		t.Fatalf("weaknesses = %+v, want evidence-preserving append", got.Weaknesses)
	}
	if len(got.LastAdvice) != 2 {
		t.Fatalf("advice = %+v, want deduped merge", got.LastAdvice)
	}

	if existing.SkillScores["redis"] != 60 || len(existing.Strengths) != 1 {
		t.Fatalf("existing memory was mutated: %+v", existing)
	}
}

func TestApplyUpdate_DeduplicatesWeaknessAndKeepsStrongerEvidence(t *testing.T) {
	oldTime := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	existing := &UserMemory{
		UserID: "u1",
		Weaknesses: []Weakness{{
			Topic:     "redis",
			Evidence:  "缓存击穿回答不完整",
			Severity:  1,
			UpdatedAt: oldTime,
		}},
		SkillScores: map[string]float64{},
	}
	update := UserMemoryUpdate{
		UserID: "u1",
		Weaknesses: []Weakness{{
			Topic:     "redis",
			Evidence:  "缓存击穿回答不完整",
			Severity:  3,
			UpdatedAt: newTime,
		}},
		UpdatedAt: newTime,
	}

	got, err := ApplyUpdate(existing, update)
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if len(got.Weaknesses) != 1 {
		t.Fatalf("weaknesses = %+v, want deduped weakness", got.Weaknesses)
	}
	if got.Weaknesses[0].Severity != 3 || !got.Weaknesses[0].UpdatedAt.Equal(newTime) {
		t.Fatalf("weakness = %+v, want stronger latest duplicate", got.Weaknesses[0])
	}
}

func TestApplyUpdate_DoesNotMoveUpdatedAtBackwards(t *testing.T) {
	newer := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	existing := &UserMemory{
		UserID:      "u1",
		SkillScores: map[string]float64{},
		UpdatedAt:   newer,
	}

	got, err := ApplyUpdate(existing, UserMemoryUpdate{
		UserID:    "u1",
		UpdatedAt: older,
	})
	if err != nil {
		t.Fatalf("apply older update: %v", err)
	}
	if !got.UpdatedAt.Equal(newer) {
		t.Fatalf("updated_at moved backwards: %v", got.UpdatedAt)
	}

	got, err = ApplyUpdate(existing, UserMemoryUpdate{UserID: "u1"})
	if err != nil {
		t.Fatalf("apply zero-time update: %v", err)
	}
	if !got.UpdatedAt.Equal(newer) {
		t.Fatalf("zero updated_at overwrote timestamp: %v", got.UpdatedAt)
	}
}

func TestBuildUpdateFromSession_DoesNotMutateSessionReportOrWorkingMemory(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	sess := &domain.Session{
		ID:            "s1",
		UserID:        "u1",
		WorkingMemory: domain.NewWorkingMemory(),
		Report: &domain.Report{
			Highlights:     []string{"  项目表达清楚  "},
			Improvements:   []string{"Redis 回答不完整"},
			NextSteps:      []string{"补 Redis"},
			SkillBreakdown: map[string]int{"redis": 45},
		},
	}
	before := mustJSON(t, sess)

	if _, err := BuildUpdateFromSession(sess, now); err != nil {
		t.Fatalf("build update: %v", err)
	}
	after := mustJSON(t, sess)
	if before != after {
		t.Fatalf("session mutated\nbefore=%s\nafter=%s", before, after)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
