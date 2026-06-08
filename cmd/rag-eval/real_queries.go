package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"interview-agent/internal/domain"
)

type exportedQuery struct {
	ID            string   `json:"id"`
	SessionID     string   `json:"session_id"`
	Query         string   `json:"query"`
	OriginalQuery string   `json:"original_query,omitempty"`
	Skill         string   `json:"skill,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	CandidateIDs  []string `json:"candidate_ids,omitempty"`
}

func runExportQueries(opts options, stdout, stderr io.Writer) int {
	if strings.TrimSpace(opts.SessionsPath) == "" {
		fmt.Fprintln(stderr, "ERROR: -sessions is required for export-queries")
		return 2
	}
	if strings.TrimSpace(opts.OutFile) == "" {
		fmt.Fprintln(stderr, "ERROR: -out-file is required for export-queries")
		return 2
	}
	sessions, err := loadSessionFile(opts.SessionsPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: load sessions: %v\n", err)
		return 2
	}
	queries := exportQueriesFromSessions(sessions)
	if err := writeJSONLFile(opts.OutFile, queries); err != nil {
		fmt.Fprintf(stderr, "ERROR: write queries: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "rag-eval export-queries: sessions=%d queries=%d out=%s\n", len(sessions), len(queries), opts.OutFile)
	return 0
}

func loadSessionFile(path string) ([]domain.Session, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadSessions(bytes.NewReader(raw))
}

func loadExportedQueryFile(path string) ([]exportedQuery, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadExportedQueries(bytes.NewReader(raw))
}

func loadSessions(r io.Reader) ([]domain.Session, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty sessions input")
	}
	var list []domain.Session
	if err := json.Unmarshal(trimmed, &list); err == nil {
		return list, nil
	}
	var one domain.Session
	if err := json.Unmarshal(trimmed, &one); err == nil && strings.TrimSpace(one.ID) != "" {
		return []domain.Session{one}, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var sess domain.Session
		if err := json.Unmarshal([]byte(line), &sess); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if strings.TrimSpace(sess.ID) != "" {
			list = append(list, sess)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no sessions")
	}
	return list, nil
}

func loadExportedQueries(r io.Reader) ([]exportedQuery, error) {
	var out []exportedQuery
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var query exportedQuery
		if err := json.Unmarshal([]byte(line), &query); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if strings.TrimSpace(query.Query) == "" {
			return nil, fmt.Errorf("line %d: query required", lineNo)
		}
		out = append(out, query)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no queries")
	}
	return out, nil
}

func exportQueriesFromSessions(sessions []domain.Session) []exportedQuery {
	out := make([]exportedQuery, 0, len(sessions))
	for _, sess := range sessions {
		if sess.RetrievalTrace == nil || strings.TrimSpace(sess.RetrievalTrace.Query) == "" {
			continue
		}
		out = append(out, exportedQuery{
			ID:            queryID(sess),
			SessionID:     sess.ID,
			Query:         sanitizeQueryText(sess.RetrievalTrace.Query),
			OriginalQuery: sanitizeQueryText(sess.RetrievalTrace.OriginalQuery),
			Skill:         firstCandidateSkill(sess.CandidatePool),
			Tags:          firstCandidateTags(sess.CandidatePool),
			CandidateIDs:  candidateIDsFromSession(sess),
		})
	}
	return out
}

func queryID(sess domain.Session) string {
	if strings.TrimSpace(sess.ID) == "" {
		return "query"
	}
	return sess.ID + ":retrieval"
}

func firstCandidateSkill(candidates []domain.Question) string {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.SkillCategory) != "" {
			return candidate.SkillCategory
		}
	}
	return ""
}

func firstCandidateTags(candidates []domain.Question) []string {
	for _, candidate := range candidates {
		if len(candidate.Tags) > 0 {
			return append([]string(nil), candidate.Tags...)
		}
	}
	return nil
}

func candidateIDsFromSession(sess domain.Session) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, candidate := range sess.CandidatePool {
		add(candidate.ID)
	}
	if sess.RetrievalTrace != nil {
		for _, item := range sess.RetrievalTrace.Final {
			add(item.ID)
		}
		for _, stage := range sess.RetrievalTrace.Stages {
			for _, item := range stage.Items {
				add(item.ID)
			}
		}
	}
	return out
}

var (
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	urlPattern    = regexp.MustCompile(`(?i)\bhttps?://[^\s]+`)
	phonePattern  = regexp.MustCompile(`\b(?:\+?86[- ]?)?1[3-9]\d{9}\b`)
	secretPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|secret|password)\s*[:=]\s*["']?[^"'\s,;]+`)
)

func sanitizeQueryText(raw string) string {
	out := strings.TrimSpace(raw)
	out = urlPattern.ReplaceAllString(out, "[REDACTED_URL]")
	out = emailPattern.ReplaceAllString(out, "[REDACTED_EMAIL]")
	out = phonePattern.ReplaceAllString(out, "[REDACTED_PHONE]")
	out = secretPattern.ReplaceAllString(out, "$1=[REDACTED_SECRET]")
	return out
}

func writeJSONLFile(path string, rows any) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeJSONL(f, rows)
}

func writeJSONL(w io.Writer, rows any) error {
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	for _, item := range list {
		if _, err := fmt.Fprintln(w, string(item)); err != nil {
			return err
		}
	}
	return nil
}
