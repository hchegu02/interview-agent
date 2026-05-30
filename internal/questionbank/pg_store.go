package questionbank

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	Pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{Pool: pool}
}

func (s *PGStore) List(ctx context.Context, filter Filter) (ListResult, error) {
	if s == nil || s.Pool == nil {
		return ListResult{}, nil
	}
	limit := normalizeLimit(filter.Limit)
	offset, err := parseCursor(filter.Cursor)
	if err != nil {
		return ListResult{}, err
	}
	where, args := buildWhere(filter)
	args = append(args, limit+1, offset)
	query := `
SELECT id, content, tags, skill_category, difficulty, expected_points, source,
       scenario, role_tags, rubric, sample_answer, follow_up_hints, locale, status,
       embedding_status, embedding_model, embedded_at, embedding_error, created_at, updated_at
FROM question_bank
` + where + `
ORDER BY skill_category, difficulty, id
LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("question bank list: %w", err)
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return ListResult{}, err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return ListResult{Items: items, NextCursor: next}, nil
}

func (s *PGStore) Get(ctx context.Context, id string) (Item, error) {
	if s == nil || s.Pool == nil || strings.TrimSpace(id) == "" {
		return Item{}, ErrNotFound
	}
	row := s.Pool.QueryRow(ctx, `
SELECT id, content, tags, skill_category, difficulty, expected_points, source,
       scenario, role_tags, rubric, sample_answer, follow_up_hints, locale, status,
       embedding_status, embedding_model, embedded_at, embedding_error, created_at, updated_at
FROM question_bank
WHERE id = $1
`, id)
	item, err := scanItem(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("question bank get: %w", err)
	}
	return item, nil
}

func (s *PGStore) Facets(ctx context.Context) (Facets, error) {
	f := Facets{
		SkillCategories: map[string]int{},
		Scenarios:       map[string]int{},
		Tags:            map[string]int{},
		Difficulties:    map[int]int{},
	}
	if s == nil || s.Pool == nil {
		return f, nil
	}
	rows, err := s.Pool.Query(ctx, `
SELECT skill_category, scenario, difficulty, tags
FROM question_bank
WHERE status = 'active'
`)
	if err != nil {
		return f, fmt.Errorf("question bank facets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var skill, scenario string
		var difficulty int
		var tags []string
		if err := rows.Scan(&skill, &scenario, &difficulty, &tags); err != nil {
			return f, fmt.Errorf("question bank facets scan: %w", err)
		}
		if skill != "" {
			f.SkillCategories[skill]++
		}
		if scenario != "" {
			f.Scenarios[scenario]++
		}
		if difficulty > 0 {
			f.Difficulties[difficulty]++
		}
		for _, tag := range tags {
			if tag != "" {
				f.Tags[tag]++
			}
		}
	}
	return f, rows.Err()
}

func (s *PGStore) Upsert(ctx context.Context, items []Item) error {
	if s == nil || s.Pool == nil || len(items) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, item := range items {
		item = normalizeItem(item)
		rubric, err := json.Marshal(item.Rubric)
		if err != nil {
			return fmt.Errorf("marshal question rubric: %w", err)
		}
		batch.Queue(`
INSERT INTO question_bank (
    id, content, tags, skill_category, difficulty, expected_points, source,
    scenario, role_tags, rubric, sample_answer, follow_up_hints, locale, status,
    embedding_status, embedding_error, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10::jsonb, $11, $12, $13, $14,
    'pending', '', now()
)
ON CONFLICT (id) DO UPDATE SET
    content = EXCLUDED.content,
    tags = EXCLUDED.tags,
    skill_category = EXCLUDED.skill_category,
    difficulty = EXCLUDED.difficulty,
    expected_points = EXCLUDED.expected_points,
    source = EXCLUDED.source,
    scenario = EXCLUDED.scenario,
    role_tags = EXCLUDED.role_tags,
    rubric = EXCLUDED.rubric,
    sample_answer = EXCLUDED.sample_answer,
    follow_up_hints = EXCLUDED.follow_up_hints,
    locale = EXCLUDED.locale,
    status = EXCLUDED.status,
    embedding_status = CASE
        WHEN question_bank.content IS DISTINCT FROM EXCLUDED.content THEN 'pending'
        ELSE question_bank.embedding_status
    END,
    embedding_error = CASE
        WHEN question_bank.content IS DISTINCT FROM EXCLUDED.content THEN ''
        ELSE question_bank.embedding_error
    END,
    updated_at = now()
`, item.ID, item.Content, item.Tags, item.SkillCategory, item.Difficulty, item.ExpectedPoints, item.Source,
			item.Scenario, item.RoleTags, string(rubric), item.SampleAnswer, item.FollowUpHints, item.Locale, item.Status)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range items {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("question bank upsert: %w", err)
		}
	}
	return nil
}

func (s *PGStore) UpsertEmbeddings(ctx context.Context, vectors []ItemEmbedding) error {
	if s == nil || s.Pool == nil || len(vectors) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, vector := range vectors {
		batch.Queue(`
UPDATE question_bank
SET embedding = $2::vector,
    embedding_status = 'embedded',
    embedding_model = $3,
    embedded_at = now(),
    embedding_error = '',
    updated_at = now()
WHERE id = $1
`, vector.ID, vectorLiteral(vector.Vector), vector.Model)
	}
	br := s.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range vectors {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("question bank embedding upsert: %w", err)
		}
	}
	return nil
}

func buildWhere(filter Filter) (string, []any) {
	var clauses []string
	var args []any
	status := filter.Status
	if status == "" {
		status = "active"
	}
	args = append(args, status)
	clauses = append(clauses, "status = $1")
	if filter.SkillCategory != "" {
		args = append(args, filter.SkillCategory)
		clauses = append(clauses, fmt.Sprintf("skill_category = $%d", len(args)))
	}
	if filter.Scenario != "" {
		args = append(args, filter.Scenario)
		clauses = append(clauses, fmt.Sprintf("scenario = $%d", len(args)))
	}
	if filter.Difficulty > 0 {
		args = append(args, filter.Difficulty)
		clauses = append(clauses, fmt.Sprintf("difficulty = $%d", len(args)))
	}
	tags := compactStrings(filter.Tags)
	if len(tags) > 0 {
		args = append(args, tags)
		clauses = append(clauses, fmt.Sprintf("tags @> $%d::text[]", len(args)))
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		clauses = append(clauses, fmt.Sprintf("(id ILIKE $%d OR content ILIKE $%d OR EXISTS (SELECT 1 FROM unnest(tags) tag WHERE tag ILIKE $%d))", n, n, n))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItems(rows pgx.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("question bank rows: %w", rows.Err())
	}
	return items, nil
}

func scanItem(row itemScanner) (Item, error) {
	var item Item
	var rubricRaw []byte
	if err := row.Scan(
		&item.ID, &item.Content, &item.Tags, &item.SkillCategory, &item.Difficulty,
		&item.ExpectedPoints, &item.Source, &item.Scenario, &item.RoleTags, &rubricRaw,
		&item.SampleAnswer, &item.FollowUpHints, &item.Locale, &item.Status,
		&item.EmbeddingStatus, &item.EmbeddingModel, &item.EmbeddedAt, &item.EmbeddingError,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Item{}, err
	}
	if len(rubricRaw) > 0 {
		_ = json.Unmarshal(rubricRaw, &item.Rubric)
	}
	return normalizeItem(item), nil
}

func vectorLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 12)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
