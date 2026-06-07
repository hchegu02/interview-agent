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
	ID              string            `json:"id"`
	Content         string            `json:"content"`
	Tags            []string          `json:"tags,omitempty"`
	SkillCategory   string            `json:"skill_category,omitempty"`
	Difficulty      int               `json:"difficulty,omitempty"`
	Source          string            `json:"source,omitempty"`
	Scenario        string            `json:"scenario,omitempty"`
	RoleTags        []string          `json:"role_tags,omitempty"`
	Locale          string            `json:"locale,omitempty"`
	Status          string            `json:"status,omitempty"`
	EmbeddingStatus string            `json:"embedding_status,omitempty"`
	EmbeddingModel  string            `json:"embedding_model,omitempty"`
	EmbeddingError  string            `json:"embedding_error,omitempty"`
	ExpectedPoints  []string          `json:"expected_points,omitempty"`
	Rubric          map[string]string `json:"rubric,omitempty"`
	SampleAnswer    string            `json:"sample_answer,omitempty"`
	FollowUpHints   []string          `json:"follow_up_hints,omitempty"`
}

type questionBankImportResponse struct {
	Job   questionbank.ImportJob    `json:"job"`
	Items []questionbank.ImportItem `json:"items,omitempty"`
}

type questionBankImportListResponse struct {
	Jobs []questionbank.ImportJob `json:"jobs"`
}

type questionBankGenerationStageResponse struct {
	Job       questionbank.GenerationJob `json:"job"`
	ImportJob questionbank.ImportJob     `json:"import_job"`
	Items     []questionbank.ImportItem  `json:"items,omitempty"`
}

type questionBankImportReviewRequest struct {
	Action  string   `json:"action"`
	ItemIDs []string `json:"item_ids"`
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

func (s *Server) createQuestionBankImport(c *gin.Context) {
	if s.questionImports == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank import service not configured"})
		return
	}
	sourceType := strings.TrimSpace(c.PostForm("source_type"))
	if sourceType == "" {
		sourceType = questionbank.ImportSourceQuestionBank
	}
	if sourceType != questionbank.ImportSourceQuestionBank && sourceType != questionbank.ImportSourceDocument {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type must be question_bank or document"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "import file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "open import file failed"})
		return
	}
	defer file.Close()

	importFile := questionbank.ImportFile{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		Reader:      file,
		Size:        fileHeader.Size,
	}
	var job questionbank.ImportJob
	if c.Query("async") == "true" || c.PostForm("async") == "true" {
		job, err = s.questionImports.EnqueueImport(c.Request.Context(), sourceType, importFile)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "job": job})
			return
		}
		c.JSON(http.StatusAccepted, questionBankImportResponse{Job: job})
		return
	}
	switch sourceType {
	case questionbank.ImportSourceQuestionBank:
		job, err = s.questionImports.ImportLocalQuestionBank(c.Request.Context(), importFile)
	case questionbank.ImportSourceDocument:
		job, err = s.questionImports.ImportDocument(c.Request.Context(), importFile)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "job": job})
		return
	}
	c.JSON(http.StatusCreated, questionBankImportResponse{Job: job})
}

func (s *Server) listQuestionBankImports(c *gin.Context) {
	if s.questionImports == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank import service not configured"})
		return
	}
	jobs, err := s.questionImports.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list question bank imports failed"})
		return
	}
	c.JSON(http.StatusOK, questionBankImportListResponse{Jobs: jobs})
}

func (s *Server) getQuestionBankImport(c *gin.Context) {
	if s.questionImports == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank import service not configured"})
		return
	}
	job, items, err := s.questionImports.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, questionbank.ErrImportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question bank import not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get question bank import failed"})
		return
	}
	c.JSON(http.StatusOK, questionBankImportResponse{Job: job, Items: items})
}

func (s *Server) reviewQuestionBankImportItems(c *gin.Context) {
	if s.questionImports == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank import service not configured"})
		return
	}
	var req questionBankImportReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review request"})
		return
	}
	action := strings.TrimSpace(req.Action)
	var (
		job   questionbank.ImportJob
		items []questionbank.ImportItem
		err   error
	)
	switch action {
	case "accept":
		job, items, err = s.questionImports.ReviewItems(c.Request.Context(), c.Param("id"), req.ItemIDs, questionbank.ImportReviewStatusAccepted)
	case "reject":
		job, items, err = s.questionImports.ReviewItems(c.Request.Context(), c.Param("id"), req.ItemIDs, questionbank.ImportReviewStatusRejected)
	case "accept_all_valid":
		job, items, err = s.questionImports.ReviewAllValidItems(c.Request.Context(), c.Param("id"), questionbank.ImportReviewStatusAccepted, false)
	case "reject_all_valid":
		job, items, err = s.questionImports.ReviewAllValidItems(c.Request.Context(), c.Param("id"), questionbank.ImportReviewStatusRejected, false)
	case "accept_complete_valid":
		job, items, err = s.questionImports.ReviewAllValidItems(c.Request.Context(), c.Param("id"), questionbank.ImportReviewStatusAccepted, true)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported review action"})
		return
	}
	if err != nil {
		if errors.Is(err, questionbank.ErrImportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question bank import not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, questionBankImportResponse{Job: job, Items: items})
}

func (s *Server) commitQuestionBankImport(c *gin.Context) {
	if s.questionImports == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank import service not configured"})
		return
	}
	var job questionbank.ImportJob
	var err error
	if c.Query("async") == "true" || c.PostForm("async") == "true" {
		job, err = s.questionImports.EnqueueCommit(c.Request.Context(), c.Param("id"))
		if err == nil {
			c.JSON(http.StatusAccepted, questionBankImportResponse{Job: job})
			return
		}
	} else {
		job, err = s.questionImports.Commit(c.Request.Context(), c.Param("id"))
	}
	if err != nil {
		if errors.Is(err, questionbank.ErrImportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question bank import not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, questionBankImportResponse{Job: job})
}

func (s *Server) createQuestionBankGenerationJob(c *gin.Context) {
	if s.questionGeneration == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank generation service not configured"})
		return
	}
	var req questionbank.GenerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid generation request"})
		return
	}
	if c.Query("async") == "true" {
		job, err := s.questionGeneration.EnqueueGenerate(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "job": job})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"job": job})
		return
	}
	job, err := s.questionGeneration.Generate(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "job": job})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"job": job})
}

func (s *Server) getQuestionBankGenerationJob(c *gin.Context) {
	if s.questionGeneration == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank generation service not configured"})
		return
	}
	job, err := s.questionGeneration.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, questionbank.ErrImportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question bank generation job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get question bank generation job failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": job})
}

func (s *Server) stageQuestionBankGenerationJob(c *gin.Context) {
	if s.questionGeneration == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "question bank generation service not configured"})
		return
	}
	job, importJob, items, err := s.questionGeneration.Stage(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, questionbank.ErrImportNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "question bank generation job not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "job": job})
		return
	}
	c.JSON(http.StatusOK, questionBankGenerationStageResponse{
		Job:       job,
		ImportJob: importJob,
		Items:     items,
	})
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
		out.EmbeddingStatus = item.EmbeddingStatus
		out.EmbeddingModel = item.EmbeddingModel
		out.EmbeddingError = item.EmbeddingError
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
