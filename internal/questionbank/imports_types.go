package questionbank

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"interview-agent/internal/embedding"
	"interview-agent/internal/llm"
	"interview-agent/internal/parser"
)

// 定义了题库导入相关的类型和服务，包括导入作业、切片、题目，以及导入服务的实现。支持从本地文件或文档生成题目，并提供异步处理和恢复机制。
const (
	ImportSourceQuestionBank = "question_bank"
	ImportSourceDocument     = "document"

	ImportStatusQueued     = "queued"
	ImportStatusCreated    = "created"
	ImportStatusParsing    = "parsing"
	ImportStatusGenerating = "generating"
	ImportStatusValidating = "validating"
	ImportStatusReady      = "ready"
	ImportStatusCommitting = "committing"
	ImportStatusCommitted  = "committed"
	ImportStatusFailed     = "failed"

	ImportItemStatusValid    = "valid"
	ImportItemStatusInvalid  = "invalid"
	ImportItemStatusRejected = "rejected"
	ImportItemStatusImported = "imported"

	ImportReviewStatusAccepted = "accepted"
	ImportReviewStatusRejected = "rejected"

	ImportAgentReviewAutoApproved     = "auto_approved"
	ImportAgentReviewNeedsHumanReview = "needs_human_review"
	ImportAgentReviewRejected         = "rejected"

	ImportMetadataCommitSummary = "commit_summary"

	localEnrichmentBatchSize = 2
)

var ErrImportNotFound = errors.New("question bank import job not found")

type ImportJob struct {
	ID            string            `json:"id"`
	SourceType    string            `json:"source_type"`
	Filename      string            `json:"filename"`
	Status        string            `json:"status"`
	OwnerID       string            `json:"owner_id,omitempty"`
	LeaseUntil    time.Time         `json:"lease_until,omitempty"`
	TotalChunks   int               `json:"total_chunks"`
	TotalItems    int               `json:"total_items"`
	ValidItems    int               `json:"valid_items"`
	InvalidItems  int               `json:"invalid_items"`
	ImportedItems int               `json:"imported_items"`
	Error         string            `json:"error,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// 题库导入切片，表示从文档中切分出的一段文本，用于生成题目。每个切片关联一个导入作业，可以包含一些元数据。
type ImportChunk struct {
	ID        string            `json:"id"`
	JobID     string            `json:"job_id"`
	Index     int               `json:"index"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type ImportItem struct {
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	ChunkID           string            `json:"chunk_id,omitempty"`
	QuestionID        string            `json:"question_id"`
	Status            string            `json:"status"`
	ReviewStatus      string            `json:"review_status"`
	AgentReviewStatus string            `json:"agent_review_status,omitempty"`
	AgentReviewReason string            `json:"agent_review_reason,omitempty"`
	Item              Item              `json:"item"`
	OriginalItem      *Item             `json:"original_item,omitempty"`
	FieldProvenance   map[string]string `json:"field_provenance,omitempty"`
	SourceProvenance  map[string]string `json:"source_provenance,omitempty"`
	Errors            []string          `json:"errors,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type ImportFile struct {
	Filename    string
	ContentType string
	Reader      io.Reader
	Size        int64
}

type ImportFileRef struct {
	Path        string
	Filename    string
	ContentType string
	Size        int64
}

type ImportSpool interface {
	Save(ctx context.Context, jobID string, file ImportFile) (ImportFileRef, error)
	Open(ctx context.Context, ref ImportFileRef) (ImportFile, func(), error)
	Delete(ctx context.Context, ref ImportFileRef) error
}

// 定义了题库导入存储接口，提供了创建、更新、获取和列出导入作业的方法，以及添加和列出切片和题目、更新题目状态、重置作业数据和尝试获取作业锁的方法。实现该接口可以使用内存、数据库或其他持久化存储。
type ImportStore interface {
	CreateJob(ctx context.Context, job ImportJob) (ImportJob, error)
	UpdateJob(ctx context.Context, job ImportJob) (ImportJob, error)
	GetJob(ctx context.Context, id string) (ImportJob, error)
	ListJobs(ctx context.Context) ([]ImportJob, error)
	AddChunks(ctx context.Context, chunks []ImportChunk) error
	ListChunks(ctx context.Context, jobID string) ([]ImportChunk, error)
	AddItems(ctx context.Context, items []ImportItem) error
	ListItems(ctx context.Context, jobID string) ([]ImportItem, error)
	UpdateItems(ctx context.Context, items []ImportItem) error
	UpdateItemReviews(ctx context.Context, jobID string, itemIDs []string, reviewStatus string) error
	ResetJobData(ctx context.Context, jobID string) error
	TryAcquireJob(ctx context.Context, jobID, ownerID string, leaseFor time.Duration) (ImportJob, bool, error)
}

type ImportServiceDeps struct {
	Imports  ImportStore
	Writer   Writer
	Parser   parser.DocumentParser
	Model    llm.ChatModel
	Embedder embedding.Embedder
	Spool    ImportSpool
	OwnerID  string
	LeaseFor time.Duration
}

type ImportService struct {
	imports  ImportStore
	writer   Writer
	parser   parser.DocumentParser
	model    llm.ChatModel
	embedder embedding.Embedder
	spool    ImportSpool
	ownerID  string
	leaseFor time.Duration
	workers  chan struct{}

	asyncMu     sync.Mutex
	asyncClosed bool
	asyncWG     sync.WaitGroup
}
