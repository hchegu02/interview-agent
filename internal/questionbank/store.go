package questionbank

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Item struct {
	ID              string            `json:"id"`
	Content         string            `json:"content"`
	Tags            []string          `json:"tags,omitempty"`
	SkillCategory   string            `json:"skill_category,omitempty"`
	Difficulty      int               `json:"difficulty,omitempty"`
	ExpectedPoints  []string          `json:"expected_points,omitempty"`
	Source          string            `json:"source,omitempty"`
	Scenario        string            `json:"scenario,omitempty"`
	RoleTags        []string          `json:"role_tags,omitempty"`
	Rubric          map[string]string `json:"rubric,omitempty"`
	SampleAnswer    string            `json:"sample_answer,omitempty"`
	FollowUpHints   []string          `json:"follow_up_hints,omitempty"`
	Locale          string            `json:"locale,omitempty"`
	Status          string            `json:"status,omitempty"`
	EmbeddingStatus string            `json:"embedding_status,omitempty"`
	EmbeddingModel  string            `json:"embedding_model,omitempty"`
	EmbeddedAt      time.Time         `json:"embedded_at,omitempty"`
	EmbeddingError  string            `json:"embedding_error,omitempty"`
	CreatedAt       time.Time         `json:"created_at,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

type Filter struct {
	Query         string
	SkillCategory string
	Scenario      string
	Difficulty    int
	Tags          []string
	Status        string
	Limit         int
	Cursor        string
}

type ListResult struct {
	Items      []Item
	NextCursor string
}

type Facets struct {
	SkillCategories map[string]int `json:"skill_categories"`
	Scenarios       map[string]int `json:"scenarios"`
	Tags            map[string]int `json:"tags"`
	Difficulties    map[int]int    `json:"difficulties"`
}

type Store interface {
	List(ctx context.Context, filter Filter) (ListResult, error)
	Get(ctx context.Context, id string) (Item, error)
	Facets(ctx context.Context) (Facets, error)
}

type Writer interface {
	Upsert(ctx context.Context, items []Item) error
}

type EmbeddingWriter interface {
	UpsertEmbeddings(ctx context.Context, vectors []ItemEmbedding) error
}

type EmbeddingFailureWriter interface {
	MarkEmbeddingsFailed(ctx context.Context, ids []string, err error) error
}

type ItemEmbedding struct {
	ID     string
	Vector []float32
	Model  string
}

type MemoryStore struct {
	items      []Item
	byID       map[string]Item
	embeddings map[string]ItemEmbedding
}

func NewMemoryStore(items []Item) *MemoryStore {
	normalized := make([]Item, 0, len(items))
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		item = normalizeItem(item)
		normalized = append(normalized, item)
		byID[item.ID] = item
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})
	return &MemoryStore{items: normalized, byID: byID, embeddings: map[string]ItemEmbedding{}}
}

func LoadSeedFile(path string) ([]Item, error) {
	if path == "" {
		path = "seeds/question_bank.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil && !filepath.IsAbs(path) {
		raw, err = os.ReadFile(filepath.Join("..", "..", path))
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("unmarshal question bank seed: %w", err)
	}
	return items, nil
}

func (s *MemoryStore) List(_ context.Context, filter Filter) (ListResult, error) {
	if s == nil {
		return ListResult{}, nil
	}
	limit := normalizeLimit(filter.Limit)
	offset, err := parseCursor(filter.Cursor)
	if err != nil {
		return ListResult{}, err
	}
	var matched []Item
	for _, item := range s.items {
		if matchesFilter(item, filter) {
			matched = append(matched, item)
		}
	}
	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	next := ""
	if end < len(matched) {
		next = strconv.Itoa(end)
	}
	return ListResult{Items: cloneItems(matched[offset:end]), NextCursor: next}, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (Item, error) {
	if s == nil || id == "" {
		return Item{}, ErrNotFound
	}
	item, ok := s.byID[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	return cloneItem(item), nil
}

func (s *MemoryStore) Facets(_ context.Context) (Facets, error) {
	f := Facets{
		SkillCategories: map[string]int{},
		Scenarios:       map[string]int{},
		Tags:            map[string]int{},
		Difficulties:    map[int]int{},
	}
	if s == nil {
		return f, nil
	}
	for _, item := range s.items {
		if item.Status != "active" {
			continue
		}
		if item.SkillCategory != "" {
			f.SkillCategories[item.SkillCategory]++
		}
		if item.Scenario != "" {
			f.Scenarios[item.Scenario]++
		}
		if item.Difficulty > 0 {
			f.Difficulties[item.Difficulty]++
		}
		for _, tag := range item.Tags {
			if tag != "" {
				f.Tags[tag]++
			}
		}
	}
	return f, nil
}

func (s *MemoryStore) Upsert(_ context.Context, items []Item) error {
	if s.byID == nil {
		s.byID = map[string]Item{}
	}
	for _, item := range items {
		item = normalizeItem(item)
		s.byID[item.ID] = cloneItem(item)
	}
	s.items = s.items[:0]
	for _, item := range s.byID {
		s.items = append(s.items, cloneItem(item))
	}
	sort.SliceStable(s.items, func(i, j int) bool {
		return s.items[i].ID < s.items[j].ID
	})
	return nil
}

func (s *MemoryStore) UpsertEmbeddings(_ context.Context, vectors []ItemEmbedding) error {
	if s.embeddings == nil {
		s.embeddings = map[string]ItemEmbedding{}
	}
	for _, vector := range vectors {
		vector.Vector = append([]float32(nil), vector.Vector...)
		s.embeddings[vector.ID] = vector
		if item, ok := s.byID[vector.ID]; ok {
			item.EmbeddingStatus = "embedded"
			item.EmbeddingModel = vector.Model
			item.EmbeddedAt = time.Now().UTC()
			item.EmbeddingError = ""
			s.byID[vector.ID] = item
		}
	}
	for i := range s.items {
		if vector, ok := s.embeddings[s.items[i].ID]; ok {
			s.items[i].EmbeddingStatus = "embedded"
			s.items[i].EmbeddingModel = vector.Model
			s.items[i].EmbeddedAt = time.Now().UTC()
			s.items[i].EmbeddingError = ""
		}
	}
	return nil
}

func (s *MemoryStore) MarkEmbeddingsFailed(_ context.Context, ids []string, err error) error {
	if s == nil || len(ids) == 0 {
		return nil
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	now := time.Now().UTC()
	for _, id := range ids {
		delete(s.embeddings, id)
		if item, ok := s.byID[id]; ok {
			item.EmbeddingStatus = "failed"
			item.EmbeddingModel = ""
			item.EmbeddedAt = time.Time{}
			item.EmbeddingError = message
			item.UpdatedAt = now
			s.byID[id] = item
		}
	}
	for i := range s.items {
		for _, id := range ids {
			if s.items[i].ID != id {
				continue
			}
			s.items[i].EmbeddingStatus = "failed"
			s.items[i].EmbeddingModel = ""
			s.items[i].EmbeddedAt = time.Time{}
			s.items[i].EmbeddingError = message
			s.items[i].UpdatedAt = now
			break
		}
	}
	return nil
}

func (s *MemoryStore) Embedding(id string) ([]float32, string, bool) {
	if s == nil || s.embeddings == nil {
		return nil, "", false
	}
	vector, ok := s.embeddings[id]
	if !ok {
		return nil, "", false
	}
	return append([]float32(nil), vector.Vector...), vector.Model, true
}

func matchesFilter(item Item, filter Filter) bool {
	status := filter.Status
	if status == "" {
		status = "active"
	}
	if item.Status != status {
		return false
	}
	if filter.SkillCategory != "" && item.SkillCategory != filter.SkillCategory {
		return false
	}
	if filter.Scenario != "" && item.Scenario != filter.Scenario {
		return false
	}
	if filter.Difficulty > 0 && item.Difficulty != filter.Difficulty {
		return false
	}
	for _, tag := range filter.Tags {
		if tag != "" && !contains(item.Tags, tag) {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(filter.Query))
	if q == "" {
		return true
	}
	haystack := strings.ToLower(item.ID + " " + item.Content + " " + strings.Join(item.Tags, " "))
	return strings.Contains(haystack, q)
}

func normalizeItem(item Item) Item {
	if item.Status == "" {
		item.Status = "active"
	}
	if item.EmbeddingStatus == "" {
		item.EmbeddingStatus = "pending"
	}
	if item.Locale == "" {
		item.Locale = "zh-CN"
	}
	if item.Source == "" {
		item.Source = "manual"
	}
	item.Tags = compactStrings(item.Tags)
	item.RoleTags = compactStrings(item.RoleTags)
	item.ExpectedPoints = compactStrings(item.ExpectedPoints)
	item.FollowUpHints = compactStrings(item.FollowUpHints)
	return item
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func parseCursor(cursor string) (int, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return n, nil
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func cloneItems(items []Item) []Item {
	out := make([]Item, len(items))
	for i := range items {
		out[i] = cloneItem(items[i])
	}
	return out
}

func cloneItem(item Item) Item {
	item.Tags = append([]string(nil), item.Tags...)
	item.RoleTags = append([]string(nil), item.RoleTags...)
	item.ExpectedPoints = append([]string(nil), item.ExpectedPoints...)
	item.FollowUpHints = append([]string(nil), item.FollowUpHints...)
	if item.Rubric != nil {
		rubric := make(map[string]string, len(item.Rubric))
		for k, v := range item.Rubric {
			rubric[k] = v
		}
		item.Rubric = rubric
	}
	return item
}
