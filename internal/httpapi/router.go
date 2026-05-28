package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"interview-agent/internal/config"
	"interview-agent/internal/parser"
	"interview-agent/internal/questionbank"
)

// Server 持有所有依赖。阶段 0 只放骨架，后续阶段往里加 service。
type Server struct {
	cfg             *config.Config
	interview       *InterviewService
	documentParser  parser.DocumentParser
	questionBank    questionbank.Store
	metricsRecorder *metricsRecorder

	// breakerState 可选注入：real 模式下接 BreakingChatModel.State，返回
	// "closed" / "open" / "half_open"。/readyz 在 open 时回报 degraded（仍 200）。
	// mock 模式或未配置时为 nil，/readyz 按 ready 应答。
	breakerState func() string
}

func NewServer(cfg *config.Config) *Server {
	return &Server{cfg: cfg, documentParser: parser.NewDispatcher(), metricsRecorder: newMetricsRecorder()}
}

func NewServerWithInterview(cfg *config.Config, interview *InterviewService) *Server {
	return &Server{cfg: cfg, interview: interview, documentParser: parser.NewDispatcher(), metricsRecorder: newMetricsRecorder()}
}

// SetBreakerState 让入口装配阶段在构造完 Server 后注入熔断器状态查询函数。
// 不要求 thread-safe：只在 main() 启动期单次调用，Router() 之前完成。
func (s *Server) SetBreakerState(fn func() string) {
	s.breakerState = fn
}

// Router 构造 Gin 引擎。
// 中间件顺序：trace id → recovery；/api/interview/{start,answer} 子组额外挂 MaxInFlight 限流。
// SSE stream 使用独立 MaxStreams 限流，避免长连接消耗短请求容量。
// /healthz、/readyz、/api/ping 和 sessions 读路径不参与背压，保证健康检查 / 纯读路径在压力下仍可用。
func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(TraceIDMiddleware(), gin.Recovery(), s.metricsRecorder.middleware())

	s.registerWebRoutes(r)

	r.GET("/healthz", s.healthz)
	r.GET("/readyz", s.readyz)
	r.GET("/metrics", s.metrics)

	api := r.Group("/api")
	{
		api.GET("/ping", s.ping)
		api.POST("/documents/parse-resume", s.parseResumeDocument)
		api.GET("/question-bank", s.listQuestionBank)
		api.GET("/question-bank/facets", s.questionBankFacets)
		api.GET("/question-bank/:id", s.getQuestionBankItem)
		api.GET("/interview/sessions", s.listInterviewSessions)
		api.GET("/interview/sessions/:session_id", s.getInterviewSession)
	}

	streaming := r.Group("/api/interview")
	streaming.Use(MaxInFlightMiddlewareWithMetrics(s.cfg.Server.MaxStreams, s.metricsRecorder, "interview_stream"))
	{
		streaming.GET("/stream", s.streamInterview)
	}

	// LLM 入口子组：start / answer 会走 Graph + LLM，必须挂背压。
	// limit <= 0 时 middleware 退化成 no-op，对单实例 dev / 测试零侵入。
	mutating := r.Group("/api/interview")
	mutating.Use(MaxInFlightMiddlewareWithMetrics(s.cfg.Server.MaxInFlight, s.metricsRecorder, "interview_mutating"))
	{
		mutating.POST("/start", s.startInterview)
		mutating.POST("/answer", s.answerInterview)
	}
	return r
}

func (s *Server) healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readyz：基础永远 ready；如果配置了 breakerState 且当前 open，
// 仍返回 200 但 status=degraded，便于运维 dashboard / k8s probe 区分。
// 故意 *不* 返 503——熔断打开期间节点会走规则降级仍能答题，
// k8s 不应把 pod 拉出 service。
func (s *Server) readyz(c *gin.Context) {
	resp := gin.H{"status": "ready"}
	if s.breakerState != nil {
		state := s.breakerState()
		if state == "open" {
			resp["status"] = "degraded"
			resp["llm_breaker"] = state
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"pong":     true,
		"llm_mode": s.cfg.LLM.Mode,
	})
}
