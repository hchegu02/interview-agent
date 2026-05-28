package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

type profileAnalyzeRequest struct {
	JDText     string `json:"jd_text" binding:"required"`
	ResumeText string `json:"resume_text" binding:"required"`
}

type profileAnalyzeResponse struct {
	JobProfile       *domain.JobProfile       `json:"job_profile,omitempty"`
	CandidateProfile *domain.CandidateProfile `json:"candidate_profile,omitempty"`
	GapReport        *domain.GapReport        `json:"gap_report,omitempty"`
	ProfileAnalysis  *domain.ProfileAnalysis  `json:"profile_analysis,omitempty"`
}

func (s *Server) analyzeProfile(c *gin.Context) {
	if s.profileAnalyzer == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "profile analyzer not configured"})
		return
	}
	var req profileAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.JDText) == "" || strings.TrimSpace(req.ResumeText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jd_text and resume_text are required"})
		return
	}
	sess, err := s.profileAnalyzer.AnalyzeProfile(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "analyze profile failed"})
		return
	}
	c.JSON(http.StatusOK, buildProfileAnalyzeResponse(sess))
}

func buildProfileAnalyzeResponse(sess *domain.Session) profileAnalyzeResponse {
	if sess == nil {
		return profileAnalyzeResponse{}
	}
	return profileAnalyzeResponse{
		JobProfile:       cloneJobProfile(sess.JobProfile),
		CandidateProfile: cloneCandidateProfile(sess.CandProfile),
		GapReport:        cloneGapReport(sess.GapReport),
		ProfileAnalysis:  cloneProfileAnalysis(sess.ProfileAnalysis),
	}
}

func cloneGapReport(r *domain.GapReport) *domain.GapReport {
	if r == nil {
		return nil
	}
	out := *r
	out.MatchedSkills = append([]string(nil), r.MatchedSkills...)
	out.MissingSkills = append([]string(nil), r.MissingSkills...)
	return &out
}

type NodeProfileAnalyzer struct {
	parseJD     graph.NodeFunc
	parseResume graph.NodeFunc
	gapAnalyze  graph.NodeFunc
	analyze     graph.NodeFunc
}

func NewNodeProfileAnalyzer(parseJD, parseResume, gapAnalyze, analyze graph.NodeFunc) *NodeProfileAnalyzer {
	return &NodeProfileAnalyzer{
		parseJD:     parseJD,
		parseResume: parseResume,
		gapAnalyze:  gapAnalyze,
		analyze:     analyze,
	}
}

func (a *NodeProfileAnalyzer) AnalyzeProfile(ctx context.Context, req profileAnalyzeRequest) (*domain.Session, error) {
	sess := &domain.Session{
		JobProfile:  &domain.JobProfile{JDRawText: req.JDText},
		CandProfile: &domain.CandidateProfile{ResumeRawText: req.ResumeText},
	}
	for _, node := range []graph.NodeFunc{a.parseJD, a.parseResume, a.gapAnalyze, a.analyze} {
		if node == nil {
			continue
		}
		if err := node(ctx, sess); err != nil {
			return nil, err
		}
	}
	return sess, nil
}
