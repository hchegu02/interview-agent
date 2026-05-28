package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/questionbank"
)

type questionBankListResponse struct {
	Items      []questionBankItem `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Limit      int                `json:"limit"`
}

type questionBankItem struct {
	ID             string            `json:"id"`
	Content        string            `json:"content"`
	Tags           []string          `json:"tags,omitempty"`
	SkillCategory  string            `json:"skill_category,omitempty"`
	Difficulty     int               `json:"difficulty,omitempty"`
	Source         string            `json:"source,omitempty"`
	Scenario       string            `json:"scenario,omitempty"`
	RoleTags       []string          `json:"role_tags,omitempty"`
	Locale         string            `json:"locale,omitempty"`
	Status         string            `json:"status,omitempty"`
	ExpectedPoints []string          `json:"expected_points,omitempty"`
	Rubric         map[string]string `json:"rubric,omitempty"`
	SampleAnswer   string            `json:"sample_answer,omitempty"`
	FollowUpHints  []string          `json:"follow_up_hints,omitempty"`
}

func (s *Server) listQuestionBank(c *gin.Context) {
	if s.questionBank == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank store not configured"})
		return
	}
	filter, err := questionBankFilterFromQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := s.questionBank.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list question bank failed"})
		return
	}
	admin := isQuestionBankAdminView(c)
	items := make([]questionBankItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, buildQuestionBankItem(item, admin))
	}
	c.JSON(http.StatusOK, questionBankListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		Limit:      normalizeQuestionBankLimit(filter.Limit),
	})
}

func (s *Server) getQuestionBankItem(c *gin.Context) {
	if s.questionBank == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank store not configured"})
		return
	}
	item, err := s.questionBank.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, questionbank.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get question failed"})
		return
	}
	c.JSON(http.StatusOK, buildQuestionBankItem(item, isQuestionBankAdminView(c)))
}

func (s *Server) questionBankFacets(c *gin.Context) {
	if s.questionBank == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank store not configured"})
		return
	}
	facets, err := s.questionBank.Facets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get question bank facets failed"})
		return
	}
	c.JSON(http.StatusOK, facets)
}

func questionBankFilterFromQuery(c *gin.Context) (questionbank.Filter, error) {
	limit, err := parseOptionalInt(c.Query("limit"))
	if err != nil {
		return questionbank.Filter{}, err
	}
	difficulty, err := parseOptionalInt(c.Query("difficulty"))
	if err != nil {
		return questionbank.Filter{}, err
	}
	filter := questionbank.Filter{
		Query:         strings.TrimSpace(c.Query("q")),
		SkillCategory: strings.TrimSpace(c.Query("skill_category")),
		Scenario:      strings.TrimSpace(c.Query("scenario")),
		Difficulty:    difficulty,
		Tags:          splitCSV(c.Query("tag")),
		Status:        strings.TrimSpace(c.Query("status")),
		Limit:         limit,
		Cursor:        strings.TrimSpace(c.Query("cursor")),
	}
	return filter, nil
}

func buildQuestionBankItem(item questionbank.Item, admin bool) questionBankItem {
	out := questionBankItem{
		ID:            item.ID,
		Content:       item.Content,
		Tags:          append([]string(nil), item.Tags...),
		SkillCategory: item.SkillCategory,
		Difficulty:    item.Difficulty,
		Source:        item.Source,
		Scenario:      item.Scenario,
		RoleTags:      append([]string(nil), item.RoleTags...),
		Locale:        item.Locale,
		Status:        item.Status,
	}
	if admin {
		out.ExpectedPoints = append([]string(nil), item.ExpectedPoints...)
		out.Rubric = item.Rubric
		out.SampleAnswer = item.SampleAnswer
		out.FollowUpHints = append([]string(nil), item.FollowUpHints...)
	}
	return out
}

func isQuestionBankAdminView(c *gin.Context) bool {
	return c.Query("view") == "admin"
}

func parseOptionalInt(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid integer query parameter")
	}
	return n, nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeQuestionBankLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
