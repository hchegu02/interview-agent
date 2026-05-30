package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"interview-agent/internal/config"
)

func TestParseResumeDocument_ReturnsParsedText(t *testing.T) {
	body, contentType := buildMultipartFile(t, "file", "resume.txt", "  张三\n\nGo 后端工程师  ")
	server := NewServer(&config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/documents/parse-resume", body)
	req.Header.Set("Content-Type", contentType)

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Filename  string            `json:"filename"`
		Text      string            `json:"text"`
		PageCount int               `json:"page_count"`
		Metadata  map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Filename != "resume.txt" {
		t.Fatalf("filename = %q", got.Filename)
	}
	if got.Text != "张三\nGo 后端工程师" {
		t.Fatalf("text = %q", got.Text)
	}
	if got.PageCount != 1 || got.Metadata["format"] != "text" {
		t.Fatalf("parse metadata = pages:%d metadata:%+v", got.PageCount, got.Metadata)
	}
}

func TestParseResumeDocument_RejectsUnsupportedType(t *testing.T) {
	body, contentType := buildMultipartFile(t, "file", "resume.png", "not a resume")
	server := NewServer(&config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/documents/parse-resume", body)
	req.Header.Set("Content-Type", contentType)

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported") {
		t.Fatalf("body should explain unsupported type: %s", rec.Body.String())
	}
}

func buildMultipartFile(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}
