package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"interview-agent/internal/config"
	"interview-agent/internal/domain"
)

type fakeProfileAnalyzer struct{}

func (fakeProfileAnalyzer) AnalyzeProfile(ctx context.Context, req profileAnalyzeRequest) (*domain.Session, error) {
	return &domain.Session{
		JobProfile: &domain.JobProfile{
			Title:     "Go 后端工程师",
			KeySkills: []string{"go", "redis"},
			JDRawText: req.JDText,
		},
		CandProfile: &domain.CandidateProfile{
			Years:         2,
			Skills:        []string{"go"},
			ResumeRawText: req.ResumeText,
		},
		GapReport: &domain.GapReport{
			MatchedSkills: []string{"go"},
			MissingSkills: []string{"redis"},
			OverlapScore:  0.5,
			Strategy:      domain.GapStrategyExplore,
		},
		ProfileAnalysis: &domain.ProfileAnalysis{
			MatchScore:          64,
			Summary:             "中等匹配，建议验证 Go 项目深度并补 Redis。",
			MatchedRequirements: []string{"go"},
			MissingRequirements: []string{"redis"},
			QuestionFocus:       []string{"go", "redis"},
		},
	}, nil
}

func TestProfileAnalyzeDoesNotRequireInterviewService(t *testing.T) {
	server := NewServer(&config.Config{})
	server.SetProfileAnalyzer(fakeProfileAnalyzer{})

	body := bytes.NewBufferString(`{"jd_text":"需要 Go Redis","resume_text":"两年 Go"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/analyze", body)
	req.Header.Set("Content-Type", "application/json")

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got profileAnalyzeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ProfileAnalysis == nil || got.ProfileAnalysis.MatchScore != 64 {
		t.Fatalf("profile analysis = %+v", got.ProfileAnalysis)
	}
	if got.GapReport == nil || len(got.GapReport.MissingSkills) != 1 {
		t.Fatalf("gap report = %+v", got.GapReport)
	}
	if got.JobProfile == nil || got.JobProfile.JDRawText == "" {
		t.Fatalf("job profile = %+v", got.JobProfile)
	}
}

func TestProfileAnalyzeRequiresAnalyzer(t *testing.T) {
	server := NewServer(&config.Config{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/profile/analyze", bytes.NewBufferString(`{"jd_text":"jd","resume_text":"resume"}`))
	req.Header.Set("Content-Type", "application/json")

	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

var _ ProfileAnalyzer = fakeProfileAnalyzer{}
