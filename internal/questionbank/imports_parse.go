package questionbank

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const questionBankImportSchemaV1 = "question_bank_import.v1"

type questionBankImportMetadata struct {
	SchemaVersion    string
	SourceRef        string
	ValidationReport string
	ReviewPolicy     string
}

func parseQuestionBankItems(filename string, raw []byte) ([]Item, error) {
	items, _, err := parseQuestionBankImportPayload(filename, raw)
	return items, err
}

func parseQuestionBankImportPayload(filename string, raw []byte) ([]Item, questionBankImportMetadata, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		items, err := parseCSVItems(raw)
		return items, questionBankImportMetadata{}, err
	case ".md", ".markdown":
		return parseMarkdownItems(raw), questionBankImportMetadata{}, nil
	default:
		meta, err := parseQuestionBankImportMetadata(raw)
		if err != nil {
			return nil, questionBankImportMetadata{}, err
		}
		items, err := parseJSONItems(raw)
		return items, meta, err
	}
}

func parseQuestionBankImportMetadata(raw []byte) (questionBankImportMetadata, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return questionBankImportMetadata{}, nil
	}
	if len(obj) == 0 {
		return questionBankImportMetadata{}, nil
	}
	var meta questionBankImportMetadata
	schemaRaw, versioned := obj["schema_version"]
	if !versioned {
		return meta, nil
	}
	var schema string
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return meta, jsonFieldError("schema_version", "$.schema_version", schemaRaw, err)
	}
	schema = strings.TrimSpace(schema)
	if schema != questionBankImportSchemaV1 {
		return meta, fmt.Errorf("unsupported question bank import schema_version %q", schema)
	}
	meta.SchemaVersion = schema
	if sourceRaw, ok := obj["source_ref"]; ok {
		var sourceRef string
		if err := json.Unmarshal(sourceRaw, &sourceRef); err != nil {
			return meta, jsonFieldError("source_ref", "$.source_ref", sourceRaw, err)
		}
		meta.SourceRef = strings.TrimSpace(sourceRef)
	}
	meta.ValidationReport = compactRawJSON(obj["validation_report"])
	meta.ReviewPolicy = compactRawJSON(obj["review_policy"])
	return meta, nil
}

func compactRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return summarizeRawJSON(raw)
	}
	return buf.String()
}

func parseJSONItems(raw []byte) ([]Item, error) {
	var items []Item
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	var wrapped struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		flexible, flexErr := parseFlexibleJSONItems(raw)
		if flexErr == nil {
			return flexible, nil
		}
		return nil, fmt.Errorf("parse question bank json: %w", errors.Join(err, flexErr))
	}
	return wrapped.Items, nil
}

func parseFlexibleJSONItems(raw []byte) ([]Item, error) {
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		var wrapped struct {
			Items []json.RawMessage `json:"items"`
		}
		if wrappedErr := json.Unmarshal(raw, &wrapped); wrappedErr != nil {
			return nil, wrappedErr
		}
		records = wrapped.Items
	}
	items := make([]Item, 0, len(records))
	for _, record := range records {
		item, err := parseFlexibleJSONItem(record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func parseFlexibleJSONItem(raw json.RawMessage) (Item, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Item{}, err
	}
	difficultyRaw := fields["difficulty"]
	rubricRaw := fields["rubric"]
	tagsRaw := fields["tags"]
	expectedPointsRaw := fields["expected_points"]
	roleTagsRaw := fields["role_tags"]
	followUpHintsRaw := fields["follow_up_hints"]
	delete(fields, "difficulty")
	delete(fields, "rubric")
	delete(fields, "tags")
	delete(fields, "expected_points")
	delete(fields, "role_tags")
	delete(fields, "follow_up_hints")

	cleaned, err := json.Marshal(fields)
	if err != nil {
		return Item{}, err
	}
	var item Item
	if err := json.Unmarshal(cleaned, &item); err != nil {
		return Item{}, err
	}
	if len(difficultyRaw) > 0 {
		difficulty, err := parseFlexibleDifficulty(difficultyRaw)
		if err != nil {
			return Item{}, flexibleFieldError("difficulty", difficultyRaw, err)
		}
		item.Difficulty = difficulty
	}
	if len(rubricRaw) > 0 {
		rubric, err := parseFlexibleRubric(rubricRaw)
		if err != nil {
			return Item{}, flexibleFieldError("rubric", rubricRaw, err)
		}
		item.Rubric = rubric
	}
	if len(tagsRaw) > 0 {
		tags, err := parseFlexibleStringList(tagsRaw)
		if err != nil {
			return Item{}, flexibleFieldError("tags", tagsRaw, err)
		}
		item.Tags = tags
	}
	if len(expectedPointsRaw) > 0 {
		expectedPoints, err := parseFlexibleStringList(expectedPointsRaw)
		if err != nil {
			return Item{}, flexibleFieldError("expected_points", expectedPointsRaw, err)
		}
		item.ExpectedPoints = expectedPoints
	}
	if len(roleTagsRaw) > 0 {
		roleTags, err := parseFlexibleStringList(roleTagsRaw)
		if err != nil {
			return Item{}, flexibleFieldError("role_tags", roleTagsRaw, err)
		}
		item.RoleTags = roleTags
	}
	if len(followUpHintsRaw) > 0 {
		followUpHints, err := parseFlexibleStringList(followUpHintsRaw)
		if err != nil {
			return Item{}, flexibleFieldError("follow_up_hints", followUpHintsRaw, err)
		}
		item.FollowUpHints = followUpHints
	}
	return item, nil
}

func flexibleFieldError(field string, raw json.RawMessage, err error) error {
	return jsonFieldError(field, "$.items[]."+field, raw, err)
}

func jsonFieldError(field, path string, raw json.RawMessage, err error) error {
	return fmt.Errorf("parse %s at %s raw=%s: %w", field, path, summarizeRawJSON(raw), err)
}

func summarizeRawJSON(raw json.RawMessage) string {
	summary := strings.Join(strings.Fields(string(raw)), " ")
	const maxRawSummary = 120
	if len(summary) > maxRawSummary {
		return summary[:maxRawSummary] + "..."
	}
	return summary
}

func parseFlexibleDifficulty(raw json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return n, nil
}

func parseFlexibleRubric(raw json.RawMessage) (map[string]string, error) {
	var rubric map[string]string
	if err := json.Unmarshal(raw, &rubric); err == nil {
		return rubric, nil
	}
	var points []string
	if err := json.Unmarshal(raw, &points); err != nil {
		var s string
		if stringErr := json.Unmarshal(raw, &s); stringErr != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		return map[string]string{"general": s}, nil
	}
	rubric = make(map[string]string, len(points))
	for i, point := range points {
		point = strings.TrimSpace(point)
		if point == "" {
			continue
		}
		rubric[fmt.Sprintf("point_%d", i+1)] = point
	}
	return rubric, nil
}

func parseFlexibleStringList(raw json.RawMessage) ([]string, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return compactImportStrings(list), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return splitImportList(s), nil
}

func compactImportStrings(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func parseCSVItems(raw []byte) ([]Item, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse question bank csv: %w", err)
	}
	if len(records) < 2 {
		return nil, errors.New("question bank csv requires a header and at least one row")
	}
	header := map[string]int{}
	for i, col := range records[0] {
		header[strings.TrimSpace(col)] = i
	}
	var items []Item
	for _, row := range records[1:] {
		get := func(name string) string {
			if i, ok := header[name]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		difficulty, _ := strconv.Atoi(get("difficulty"))
		items = append(items, Item{
			ID:             get("id"),
			Content:        get("content"),
			Tags:           splitImportList(get("tags")),
			SkillCategory:  get("skill_category"),
			Difficulty:     difficulty,
			ExpectedPoints: splitImportList(get("expected_points")),
			Source:         get("source"),
			Scenario:       get("scenario"),
			RoleTags:       splitImportList(get("role_tags")),
			SampleAnswer:   get("sample_answer"),
			FollowUpHints:  splitImportList(get("follow_up_hints")),
			Locale:         get("locale"),
			Status:         get("status"),
		})
	}
	return items, nil
}

func parseMarkdownItems(raw []byte) []Item {
	blocks := splitMarkdownQuestionBlocks(string(raw))
	items := make([]Item, 0, len(blocks))
	for i, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		title := strings.Trim(strings.TrimSpace(lines[0]), "# ")
		content := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		if content == "" {
			content = title
		}
		items = append(items, Item{
			ID:         importGeneratedID("md", fmt.Sprintf("%d:%s", i, title)),
			Content:    content,
			Tags:       []string{"imported"},
			Difficulty: 3,
			Source:     "import:markdown",
		})
	}
	return items
}

func splitMarkdownQuestionBlocks(raw string) []string {
	var blocks []string
	var current strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "##") && current.Len() > 0 {
			blocks = append(blocks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func splitImportList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '，' || r == '；' || r == '、'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
