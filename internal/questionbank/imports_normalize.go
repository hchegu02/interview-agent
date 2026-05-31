package questionbank

import (
	"fmt"
	"strings"
	"time"
)

func normalizeImportedItem(item Item) Item {
	item.Content = strings.TrimSpace(item.Content)
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = importGeneratedID("qb", item.Content)
	}
	if item.Difficulty == 0 {
		item.Difficulty = 3
	}
	if item.SkillCategory == "" {
		item.SkillCategory = "general"
	}
	if item.Source == "" {
		item.Source = "import"
	}
	return normalizeItem(item)
}

func validateImportedItem(item Item) []string {
	var errs []string
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, "id is required")
	}
	if strings.TrimSpace(item.Content) == "" {
		errs = append(errs, "content is required")
	}
	if item.Difficulty < 1 || item.Difficulty > 5 {
		errs = append(errs, "difficulty must be between 1 and 5")
	}
	return errs
}

func newImportJob(sourceType, filename string) ImportJob {
	now := time.Now().UTC()
	return ImportJob{
		ID:         importGeneratedID("imp", fmt.Sprintf("%s:%s:%d", sourceType, filename, now.UnixNano())),
		SourceType: sourceType,
		Filename:   filename,
		Status:     ImportStatusCreated,
		Metadata:   map[string]string{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func buildImportChunks(jobID, text string) []ImportChunk {
	const maxRunes = 3500
	runes := []rune(text)
	var chunks []ImportChunk
	for start, index := 0, 0; start < len(runes); index++ {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		content := strings.TrimSpace(string(runes[start:end]))
		if content != "" {
			chunks = append(chunks, ImportChunk{
				ID:        fmt.Sprintf("%s:chunk:%03d", jobID, index),
				JobID:     jobID,
				Index:     index,
				Content:   content,
				Metadata:  map[string]string{},
				CreatedAt: time.Now().UTC(),
			})
		}
		start = end
	}
	return chunks
}

func embedText(item Item) string {
	var sb strings.Builder
	sb.WriteString(item.Content)
	if len(item.Tags) > 0 {
		sb.WriteString("\nTags: ")
		sb.WriteString(strings.Join(item.Tags, ", "))
	}
	if item.SkillCategory != "" {
		sb.WriteString("\nCategory: ")
		sb.WriteString(item.SkillCategory)
	}
	return sb.String()
}
