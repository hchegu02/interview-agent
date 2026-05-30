package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/parser"
)

type parseDocumentResponse struct {
	Filename  string            `json:"filename"`
	Text      string            `json:"text"`
	PageCount int               `json:"page_count"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (s *Server) parseResumeDocument(c *gin.Context) {
	if s.documentParser == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "document parser not configured"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "open resume file failed"})
		return
	}
	defer file.Close()

	doc, err := s.documentParser.Parse(c.Request.Context(), parser.Source{
		Data: file,
		Size: fileHeader.Size,
	}, parser.Hint{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
	}, parser.LimitResume)
	if err != nil {
		s.recordDocumentParseMetric(err)
		writeDocumentParseError(c, err)
		return
	}
	s.recordDocumentParseMetric(nil)

	c.JSON(http.StatusOK, parseDocumentResponse{
		Filename:  fileHeader.Filename,
		Text:      doc.Text,
		PageCount: doc.PageCount,
		Metadata:  doc.Metadata,
	})
}

func (s *Server) recordDocumentParseMetric(err error) {
	status := "ok"
	switch {
	case err == nil:
	case errors.Is(err, parser.ErrUnsupportedType):
		status = "unsupported_type"
	case errors.Is(err, parser.ErrTooLarge):
		status = "too_large"
	case errors.Is(err, parser.ErrEmptyDocument):
		status = "empty_document"
	case errors.Is(err, parser.ErrInvalidFormat):
		status = "invalid_format"
	case errors.Is(err, parser.ErrTooManyPages):
		status = "too_many_pages"
	default:
		status = "error"
	}
	s.metricsRecorder.recordParserDocument(status)
}

func writeDocumentParseError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, parser.ErrUnsupportedType):
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "unsupported resume file type; please upload PDF, DOCX, TXT, or Markdown"})
	case errors.Is(err, parser.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "resume file is too large"})
	case errors.Is(err, parser.ErrEmptyDocument):
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume document is empty or contains no extractable text"})
	case errors.Is(err, parser.ErrInvalidFormat):
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume document format is invalid"})
	case errors.Is(err, parser.ErrTooManyPages):
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume document has too many pages"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parse resume document failed"})
	}
}
