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

func parseQuestionBankItems(filename string, raw []byte) ([]Item, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		return parseCSVItems(raw)
	case ".md", ".markdown":
		return parseMarkdownItems(raw), nil
	default:
		return parseJSONItems(raw)
	}
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
		return nil, fmt.Errorf("parse question bank json: %w", err)
	}
	return wrapped.Items, nil
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
		return r == ',' || r == ';' || r == '|' || r == '，' || r == '、'
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
