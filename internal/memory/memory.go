// Package memory 定义跨 Session 的长期用户画像。
//
// 这里不要保存当前面试流程状态；Session / WorkingMemory 仍然负责单次面试事实。
// 本包只接收报告沉淀出的增量信号，并为后续数据库 Store、动态难度和复习规划提供稳定边界。
package memory

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"interview-agent/internal/domain"
)

var (
	ErrUserMemoryNotFound = errors.New("user memory not found")
	ErrInvalidMemoryInput = errors.New("invalid memory input")
	ErrUserMemoryConflict = errors.New("user memory write conflict")
)

type UserMemory struct {
	UserID      string             `json:"user_id"`
	Strengths   []string           `json:"strengths"`
	Weaknesses  []Weakness         `json:"weaknesses"`
	SkillScores map[string]float64 `json:"skill_scores"`
	LastAdvice  []string           `json:"last_advice"`
	UpdatedAt   time.Time          `json:"updated_at"`
	RowVersion  int64              `json:"-"`
}

type Weakness struct {
	Topic     string    `json:"topic"`
	Evidence  string    `json:"evidence"`
	Severity  int       `json:"severity"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserMemoryUpdate struct {
	UserID      string
	Strengths   []string
	Weaknesses  []Weakness
	SkillScores map[string]float64
	LastAdvice  []string
	UpdatedAt   time.Time
}

type Store interface {
	GetUserMemory(ctx context.Context, userID string) (*UserMemory, error)
	UpsertUserMemory(ctx context.Context, memory *UserMemory) error
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]*UserMemory
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]*UserMemory{}}
}

func (s *MemoryStore) GetUserMemory(_ context.Context, userID string) (*UserMemory, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: store is nil", ErrUserMemoryNotFound)
	}
	userID = strings.TrimSpace(userID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	mem, ok := s.items[userID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUserMemoryNotFound, userID)
	}
	return cloneUserMemory(mem), nil
}

func (s *MemoryStore) UpsertUserMemory(_ context.Context, memory *UserMemory) error {
	if s == nil {
		return fmt.Errorf("%w: store is nil", ErrInvalidMemoryInput)
	}
	if memory == nil {
		return fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	userID := strings.TrimSpace(memory.UserID)
	if userID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	cloned := cloneUserMemory(memory)
	cloned.UserID = userID
	if cloned.RowVersion <= 0 {
		cloned.RowVersion = 1
	} else {
		cloned.RowVersion++
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[userID] = cloned
	return nil
}

func BuildUpdateFromSession(sess *domain.Session, now time.Time) (UserMemoryUpdate, error) {
	if sess == nil {
		return UserMemoryUpdate{}, fmt.Errorf("%w: session is nil", ErrInvalidMemoryInput)
	}
	userID := strings.TrimSpace(sess.UserID)
	if userID == "" {
		return UserMemoryUpdate{}, fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	if sess.Report == nil {
		return UserMemoryUpdate{}, fmt.Errorf("%w: report is required", ErrInvalidMemoryInput)
	}

	report := sess.Report
	update := UserMemoryUpdate{
		UserID:      userID,
		Strengths:   cleanStrings(report.Highlights),
		SkillScores: map[string]float64{},
		LastAdvice:  cleanStrings(report.NextSteps),
		UpdatedAt:   now,
	}

	for skill, score := range report.SkillBreakdown {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		update.SkillScores[skill] = clampScore(float64(score))
		if score < 60 {
			update.Weaknesses = append(update.Weaknesses, Weakness{
				Topic:     skill,
				Evidence:  fmt.Sprintf("%s score %d", skill, score),
				Severity:  severityFromScore(score),
				UpdatedAt: now,
			})
		}
	}
	for _, text := range cleanStrings(report.Improvements) {
		update.Weaknesses = append(update.Weaknesses, Weakness{
			Topic:     inferTopic(text),
			Evidence:  text,
			Severity:  2,
			UpdatedAt: now,
		})
	}
	for _, item := range report.DrillPlan {
		skill := strings.TrimSpace(item.Skill)
		reason := strings.TrimSpace(item.Reason)
		if skill == "" && reason == "" {
			continue
		}
		if reason == "" {
			update.LastAdvice = append(update.LastAdvice, fmt.Sprintf("练习 %s", skill))
			continue
		}
		if skill == "" {
			update.LastAdvice = append(update.LastAdvice, reason)
			continue
		}
		update.LastAdvice = append(update.LastAdvice, fmt.Sprintf("%s: %s", skill, reason))
	}
	update.LastAdvice = uniqueStrings(update.LastAdvice)
	sortWeaknesses(update.Weaknesses)
	return update, nil
}

func ApplyUpdate(existing *UserMemory, update UserMemoryUpdate) (*UserMemory, error) {
	userID := strings.TrimSpace(update.UserID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidMemoryInput)
	}
	out := cloneUserMemory(existing)
	if out == nil {
		out = &UserMemory{UserID: userID, SkillScores: map[string]float64{}}
	}
	if out.UserID == "" {
		out.UserID = userID
	}
	if out.UserID != userID {
		return nil, fmt.Errorf("%w: user_id mismatch", ErrInvalidMemoryInput)
	}
	if out.SkillScores == nil {
		out.SkillScores = map[string]float64{}
	}

	out.Strengths = uniqueStrings(append(out.Strengths, update.Strengths...))
	out.LastAdvice = uniqueStrings(append(out.LastAdvice, update.LastAdvice...))
	out.Weaknesses = append(out.Weaknesses, cloneWeaknesses(update.Weaknesses)...)
	out.Weaknesses = dedupeWeaknesses(out.Weaknesses)
	sortWeaknesses(out.Weaknesses)
	for skill, score := range update.SkillScores {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		score = clampScore(score)
		if old, ok := out.SkillScores[skill]; ok {
			out.SkillScores[skill] = (old + score) / 2
		} else {
			out.SkillScores[skill] = score
		}
	}
	if shouldAdvanceTime(out.UpdatedAt, update.UpdatedAt) {
		out.UpdatedAt = update.UpdatedAt
	}
	return out, nil
}

func cloneUserMemory(in *UserMemory) *UserMemory {
	if in == nil {
		return nil
	}
	out := *in
	out.Strengths = append([]string(nil), in.Strengths...)
	out.Weaknesses = cloneWeaknesses(in.Weaknesses)
	out.LastAdvice = append([]string(nil), in.LastAdvice...)
	out.SkillScores = map[string]float64{}
	for k, v := range in.SkillScores {
		out.SkillScores[k] = v
	}
	return &out
}

func cloneWeaknesses(in []Weakness) []Weakness {
	if len(in) == 0 {
		return nil
	}
	out := make([]Weakness, len(in))
	copy(out, in)
	return out
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func severityFromScore(score int) int {
	switch {
	case score < 40:
		return 3
	case score < 60:
		return 2
	default:
		return 1
	}
}

func inferTopic(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "general"
	}
	for _, sep := range []string{" ", "，", "。", ":", "："} {
		if idx := strings.Index(text, sep); idx > 0 {
			return text[:idx]
		}
	}
	if len([]rune(text)) > 12 {
		return string([]rune(text)[:12])
	}
	return text
}

func sortWeaknesses(items []Weakness) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Severity != items[j].Severity {
			return items[i].Severity > items[j].Severity
		}
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		if items[i].Topic != items[j].Topic {
			return items[i].Topic < items[j].Topic
		}
		return items[i].Evidence < items[j].Evidence
	})
}

func dedupeWeaknesses(items []Weakness) []Weakness {
	if len(items) == 0 {
		return nil
	}
	type indexed struct {
		item  Weakness
		index int
	}
	merged := map[string]indexed{}
	for i, item := range items {
		key := weaknessKey(item)
		if key == "" {
			continue
		}
		current, ok := merged[key]
		if !ok {
			merged[key] = indexed{item: item, index: i}
			continue
		}
		current.item = mergeWeakness(current.item, item)
		merged[key] = current
	}
	out := make([]indexed, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].index < out[j].index
	})
	result := make([]Weakness, 0, len(out))
	for _, item := range out {
		result = append(result, item.item)
	}
	return result
}

func weaknessKey(item Weakness) string {
	topic := strings.TrimSpace(item.Topic)
	evidence := strings.TrimSpace(item.Evidence)
	if topic == "" && evidence == "" {
		return ""
	}
	return topic + "\x00" + evidence
}

func mergeWeakness(a, b Weakness) Weakness {
	if b.Severity > a.Severity {
		a.Severity = b.Severity
	}
	if shouldAdvanceTime(a.UpdatedAt, b.UpdatedAt) {
		a.UpdatedAt = b.UpdatedAt
	}
	return a
}

func shouldAdvanceTime(current, next time.Time) bool {
	if next.IsZero() {
		return false
	}
	return current.IsZero() || next.After(current)
}
