package questionbank

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	ID              string            `json:"id"`
	JobID           string            `json:"job_id"`
	ChunkID         string            `json:"chunk_id,omitempty"`
	QuestionID      string            `json:"question_id"`
	Status          string            `json:"status"`
	ReviewStatus    string            `json:"review_status"`
	Item            Item              `json:"item"`
	OriginalItem    *Item             `json:"original_item,omitempty"`
	FieldProvenance map[string]string `json:"field_provenance,omitempty"`
	Errors          []string          `json:"errors,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
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
}

func NewImportService(deps ImportServiceDeps) *ImportService {
	spool := deps.Spool
	if spool == nil {
		spool = NewLocalImportSpool(filepath.Join(os.TempDir(), "interview-agent-import-spool"))
	}
	ownerID := strings.TrimSpace(deps.OwnerID)
	if ownerID == "" {
		ownerID = importGeneratedID("owner", fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	leaseFor := deps.LeaseFor
	if leaseFor <= 0 {
		leaseFor = 2 * time.Minute
	}
	return &ImportService{
		imports:  deps.Imports,
		writer:   deps.Writer,
		parser:   deps.Parser,
		model:    deps.Model,
		embedder: deps.Embedder,
		spool:    spool,
		ownerID:  ownerID,
		leaseFor: leaseFor,
		workers:  make(chan struct{}, 2),
	}
}

func (s *ImportService) ImportLocalQuestionBank(ctx context.Context, file ImportFile) (ImportJob, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, errors.New("question bank import store not configured")
	}
	job, err := s.imports.CreateJob(ctx, newImportJob(ImportSourceQuestionBank, file.Filename))
	if err != nil {
		return ImportJob{}, err
	}
	return s.processLocalQuestionBank(ctx, job, file)
}

func (s *ImportService) processLocalQuestionBank(ctx context.Context, job ImportJob, file ImportFile) (ImportJob, error) {
	job.Status = ImportStatusParsing
	job, _ = s.imports.UpdateJob(ctx, job)

	raw, err := io.ReadAll(file.Reader)
	if err != nil {
		return s.failJob(ctx, job, fmt.Errorf("read import file: %w", err))
	}
	items, err := parseQuestionBankItems(file.Filename, raw)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	originals := cloneImportItems(items)
	items, provenances, err := s.enrichLocalItems(ctx, items)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	return s.stageItemsWithOriginalsAndProvenance(ctx, job, "", items, originals, provenances)
}

func (s *ImportService) ImportDocument(ctx context.Context, file ImportFile) (ImportJob, error) {
	if s == nil || s.imports == nil || s.parser == nil || s.model == nil {
		return ImportJob{}, errors.New("document import requires parser and llm model")
	}
	job, err := s.imports.CreateJob(ctx, newImportJob(ImportSourceDocument, file.Filename))
	if err != nil {
		return ImportJob{}, err
	}
	return s.processDocument(ctx, job, file)
}

func (s *ImportService) processDocument(ctx context.Context, job ImportJob, file ImportFile) (ImportJob, error) {
	job.Status = ImportStatusParsing
	job, _ = s.imports.UpdateJob(ctx, job)

	raw, err := io.ReadAll(file.Reader)
	if err != nil {
		return s.failJob(ctx, job, fmt.Errorf("read document import file: %w", err))
	}
	doc, err := s.parser.Parse(ctx, parser.Source{Data: bytes.NewReader(raw), Size: int64(len(raw))}, parser.Hint{
		Filename:    file.Filename,
		ContentType: file.ContentType,
	}, parser.LimitQuestionBankImport)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	chunks := buildImportChunks(job.ID, doc.Text)
	if err := s.imports.AddChunks(ctx, chunks); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.TotalChunks = len(chunks)
	job.Status = ImportStatusGenerating
	job, _ = s.imports.UpdateJob(ctx, job)

	for _, chunk := range chunks {
		items, err := s.generateItems(ctx, chunk.Content)
		if err != nil {
			return s.failJob(ctx, job, err)
		}
		job, err = s.stageItems(ctx, job, chunk.ID, items)
		if err != nil {
			return job, err
		}
	}
	return job, nil
}

func (s *ImportService) EnqueueImport(ctx context.Context, sourceType string, file ImportFile) (ImportJob, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, errors.New("question bank import store not configured")
	}
	job, err := s.imports.CreateJob(ctx, newImportJob(sourceType, file.Filename))
	if err != nil {
		return ImportJob{}, err
	}
	ref, err := s.saveImportPayload(ctx, job.ID, file)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	job.Metadata["spool_path"] = ref.Path
	job.Metadata["content_type"] = ref.ContentType
	job.Metadata["size"] = strconv.FormatInt(ref.Size, 10)
	job.Status = ImportStatusQueued
	job, err = s.imports.UpdateJob(ctx, job)
	if err != nil {
		return ImportJob{}, err
	}
	s.runAsync(func() {
		bg := context.Background()
		leased, ok, err := s.imports.TryAcquireJob(bg, job.ID, s.ownerID, s.leaseFor)
		if err != nil {
			_, _ = s.failJob(bg, job, err)
			return
		}
		if !ok {
			return
		}
		job = leased
		task, closeFn, err := s.openImportPayload(bg, job)
		if err != nil {
			_, _ = s.failJob(bg, job, err)
			return
		}
		defer func() {
			if s.spool != nil {
				_ = s.spool.Delete(bg, ref)
			}
		}()
		defer closeFn()
		switch sourceType {
		case ImportSourceQuestionBank:
			_, _ = s.processLocalQuestionBank(bg, job, task)
		case ImportSourceDocument:
			_, _ = s.processDocument(bg, job, task)
		default:
			_, _ = s.failJob(bg, job, fmt.Errorf("unsupported import source_type %q", sourceType))
		}
	})
	return job, nil
}

func (s *ImportService) saveImportPayload(ctx context.Context, jobID string, file ImportFile) (ImportFileRef, error) {
	if s.spool != nil {
		return s.spool.Save(ctx, jobID, file)
	}
	raw, err := io.ReadAll(file.Reader)
	if err != nil {
		return ImportFileRef{}, fmt.Errorf("read import file: %w", err)
	}
	return ImportFileRef{
		Filename:    file.Filename,
		ContentType: file.ContentType,
		Size:        int64(len(raw)),
	}, nil
}

func (s *ImportService) openImportPayload(ctx context.Context, job ImportJob) (ImportFile, func(), error) {
	if s.spool != nil {
		size, _ := strconv.ParseInt(job.Metadata["size"], 10, 64)
		return s.spool.Open(ctx, ImportFileRef{
			Path:        job.Metadata["spool_path"],
			Filename:    job.Filename,
			ContentType: job.Metadata["content_type"],
			Size:        size,
		})
	}
	return ImportFile{}, func() {}, errors.New("import spool not configured")
}

func (s *ImportService) EnqueueCommit(ctx context.Context, jobID string) (ImportJob, error) {
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, err
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	job.Status = ImportStatusQueued
	job, err = s.imports.UpdateJob(ctx, job)
	if err != nil {
		return ImportJob{}, err
	}
	s.runAsync(func() {
		bg := context.Background()
		if _, ok, err := s.imports.TryAcquireJob(bg, jobID, s.ownerID, s.leaseFor); err != nil || !ok {
			return
		}
		_, _ = s.commitReadyJob(bg, jobID)
	})
	return job, nil
}

func (s *ImportService) runAsync(fn func()) {
	go func() {
		s.workers <- struct{}{}
		defer func() { <-s.workers }()
		fn()
	}()
}

func (s *ImportService) RecoverPendingJobs(ctx context.Context) (int, error) {
	if s == nil || s.imports == nil {
		return 0, errors.New("question bank import store not configured")
	}
	jobs, err := s.imports.ListJobs(ctx)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, job := range jobs {
		if !isRecoverableImportStatus(job.Status) {
			continue
		}
		job := job
		switch {
		case job.Status == ImportStatusCommitting:
			leased, ok, err := s.imports.TryAcquireJob(ctx, job.ID, s.ownerID, s.leaseFor)
			if err != nil {
				return recovered, err
			}
			if !ok {
				continue
			}
			recovered++
			s.runAsync(func() {
				_, _ = s.commitReadyJob(context.Background(), leased.ID)
			})
		case job.Metadata["spool_path"] != "":
			leased, ok, err := s.imports.TryAcquireJob(ctx, job.ID, s.ownerID, s.leaseFor)
			if err != nil {
				return recovered, err
			}
			if !ok {
				continue
			}
			recovered++
			s.runAsync(func() {
				_, _ = s.resumeImportJob(context.Background(), leased)
			})
		default:
			recovered++
			_, _ = s.failJob(ctx, job, errors.New("cannot recover import job without spool_path"))
		}
	}
	return recovered, nil
}

func isRecoverableImportStatus(status string) bool {
	switch status {
	case ImportStatusQueued, ImportStatusParsing, ImportStatusGenerating, ImportStatusValidating, ImportStatusCommitting:
		return true
	default:
		return false
	}
}

func (s *ImportService) resumeImportJob(ctx context.Context, job ImportJob) (ImportJob, error) {
	if err := s.imports.ResetJobData(ctx, job.ID); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.TotalChunks = 0
	job.TotalItems = 0
	job.ValidItems = 0
	job.InvalidItems = 0
	job.ImportedItems = 0
	job.Error = ""
	job.Status = ImportStatusQueued
	job, _ = s.imports.UpdateJob(ctx, job)
	task, closeFn, err := s.openImportPayload(ctx, job)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	defer func() {
		if s.spool != nil {
			_ = s.spool.Delete(ctx, ImportFileRef{
				Path:        job.Metadata["spool_path"],
				Filename:    job.Filename,
				ContentType: job.Metadata["content_type"],
			})
		}
	}()
	defer closeFn()
	switch job.SourceType {
	case ImportSourceQuestionBank:
		return s.processLocalQuestionBank(ctx, job, task)
	case ImportSourceDocument:
		return s.processDocument(ctx, job, task)
	default:
		return s.failJob(ctx, job, fmt.Errorf("unsupported import source_type %q", job.SourceType))
	}
}

func (s *ImportService) Commit(ctx context.Context, jobID string) (ImportJob, error) {
	if s == nil || s.imports == nil || s.writer == nil {
		return ImportJob{}, errors.New("question bank import commit not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, err
	}
	if job.Status == ImportStatusCommitted {
		return job, nil
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	return s.commitReadyJob(ctx, jobID)
}

func (s *ImportService) ReviewItems(ctx context.Context, jobID string, itemIDs []string, reviewStatus string) (ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, nil, errors.New("question bank import review not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, nil, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	if reviewStatus != ImportReviewStatusAccepted && reviewStatus != ImportReviewStatusRejected {
		return ImportJob{}, nil, fmt.Errorf("unsupported import review_status %q", reviewStatus)
	}
	if len(itemIDs) == 0 {
		return ImportJob{}, nil, errors.New("import review requires at least one item id")
	}
	if err := s.imports.UpdateItemReviews(ctx, jobID, itemIDs, reviewStatus); err != nil {
		return ImportJob{}, nil, err
	}
	items, err := s.imports.ListItems(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	return job, items, nil
}

func (s *ImportService) ReviewAllValidItems(ctx context.Context, jobID string, reviewStatus string, completeOnly bool) (ImportJob, []ImportItem, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, nil, errors.New("question bank import review not configured")
	}
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	if job.Status != ImportStatusReady {
		return ImportJob{}, nil, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	if reviewStatus != ImportReviewStatusAccepted && reviewStatus != ImportReviewStatusRejected {
		return ImportJob{}, nil, fmt.Errorf("unsupported import review_status %q", reviewStatus)
	}
	items, err := s.imports.ListItems(ctx, jobID)
	if err != nil {
		return ImportJob{}, nil, err
	}
	var ids []string
	for _, item := range items {
		if item.Status != ImportItemStatusValid {
			continue
		}
		if completeOnly && !importItemHasCompleteReviewFields(item.Item) {
			continue
		}
		ids = append(ids, item.ID)
	}
	if len(ids) > 0 {
		if err := s.imports.UpdateItemReviews(ctx, jobID, ids, reviewStatus); err != nil {
			return ImportJob{}, nil, err
		}
		items, err = s.imports.ListItems(ctx, jobID)
		if err != nil {
			return ImportJob{}, nil, err
		}
	}
	return job, items, nil
}

func (s *ImportService) commitReadyJob(ctx context.Context, jobID string) (ImportJob, error) {
	job, err := s.imports.GetJob(ctx, jobID)
	if err != nil {
		return ImportJob{}, err
	}
	if job.Status == ImportStatusCommitted {
		return job, nil
	}
	if job.Status != ImportStatusReady && job.Status != ImportStatusQueued && job.Status != ImportStatusCommitting {
		return ImportJob{}, fmt.Errorf("import job %s is %s, not ready", job.ID, job.Status)
	}
	job.Status = ImportStatusCommitting
	job, _ = s.imports.UpdateJob(ctx, job)

	importItems, err := s.imports.ListItems(ctx, job.ID)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	var items []Item
	var updated []ImportItem
	for _, item := range importItems {
		if item.Status != ImportItemStatusValid || !importItemAccepted(item) {
			continue
		}
		items = append(items, item.Item)
		item.Status = ImportItemStatusImported
		item.UpdatedAt = time.Now().UTC()
		updated = append(updated, item)
	}
	if err := s.writer.Upsert(ctx, items); err != nil {
		return s.failJob(ctx, job, err)
	}
	if err := s.embedCommittedItems(ctx, items); err != nil {
		return s.failJob(ctx, job, err)
	}
	if err := s.imports.UpdateItems(ctx, updated); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.Status = ImportStatusCommitted
	job.ImportedItems = len(items)
	return s.imports.UpdateJob(ctx, job)
}

func (s *ImportService) embedCommittedItems(ctx context.Context, items []Item) error {
	if s.embedder == nil || len(items) == 0 {
		return nil
	}
	writer, ok := s.writer.(EmbeddingWriter)
	if !ok {
		return nil
	}
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = embedText(item)
	}
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed imported questions: %w", err)
	}
	if len(vectors) != len(items) {
		return fmt.Errorf("embed imported questions: got %d vectors, want %d", len(vectors), len(items))
	}
	out := make([]ItemEmbedding, 0, len(items))
	for i, item := range items {
		if len(vectors[i]) != s.embedder.Dimension() {
			return fmt.Errorf("%w: question %s vector dim %d, want %d", embedding.ErrDimensionMismatch, item.ID, len(vectors[i]), s.embedder.Dimension())
		}
		out = append(out, ItemEmbedding{
			ID:     item.ID,
			Vector: vectors[i],
			Model:  s.embedder.Name(),
		})
	}
	return writer.UpsertEmbeddings(ctx, out)
}

func (s *ImportService) Get(ctx context.Context, id string) (ImportJob, []ImportItem, error) {
	job, err := s.imports.GetJob(ctx, id)
	if err != nil {
		return ImportJob{}, nil, err
	}
	items, err := s.imports.ListItems(ctx, id)
	return job, items, err
}

func (s *ImportService) List(ctx context.Context) ([]ImportJob, error) {
	if s == nil || s.imports == nil {
		return nil, errors.New("question bank import store not configured")
	}
	return s.imports.ListJobs(ctx)
}

func (s *ImportService) stageItems(ctx context.Context, job ImportJob, chunkID string, items []Item) (ImportJob, error) {
	return s.stageItemsWithOriginals(ctx, job, chunkID, items, nil)
}

func (s *ImportService) stageItemsWithOriginals(ctx context.Context, job ImportJob, chunkID string, items []Item, originals []Item) (ImportJob, error) {
	return s.stageItemsWithOriginalsAndProvenance(ctx, job, chunkID, items, originals, nil)
}

func (s *ImportService) stageItemsWithOriginalsAndProvenance(ctx context.Context, job ImportJob, chunkID string, items []Item, originals []Item, provenances []map[string]string) (ImportJob, error) {
	job.Status = ImportStatusValidating
	job, _ = s.imports.UpdateJob(ctx, job)
	staged := make([]ImportItem, 0, len(items))
	for i, item := range items {
		var original *Item
		if i < len(originals) {
			originalItem := cloneItem(originals[i])
			original = &originalItem
		}
		parsedItem := item
		item = normalizeImportedItem(item)
		fieldProvenance := importFieldProvenance(parsedItem, item, original)
		if i < len(provenances) {
			for field, source := range provenances[i] {
				fieldProvenance[field] = source
			}
		}
		errs := validateImportedItem(item)
		status := ImportItemStatusValid
		if len(errs) > 0 {
			status = ImportItemStatusInvalid
		}
		staged = append(staged, ImportItem{
			ID:              job.ID + ":" + item.ID,
			JobID:           job.ID,
			ChunkID:         chunkID,
			QuestionID:      item.ID,
			Status:          status,
			ReviewStatus:    ImportReviewStatusAccepted,
			Item:            item,
			OriginalItem:    original,
			FieldProvenance: fieldProvenance,
			Errors:          errs,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		})
		if status == ImportItemStatusValid {
			job.ValidItems++
		} else {
			job.InvalidItems++
		}
	}
	if err := s.imports.AddItems(ctx, staged); err != nil {
		return s.failJob(ctx, job, err)
	}
	job.TotalItems += len(staged)
	job.Status = ImportStatusReady
	return s.imports.UpdateJob(ctx, job)
}

func (s *ImportService) failJob(ctx context.Context, job ImportJob, err error) (ImportJob, error) {
	job.Status = ImportStatusFailed
	job.Error = err.Error()
	updated, updateErr := s.imports.UpdateJob(ctx, job)
	if updateErr != nil {
		return updated, updateErr
	}
	return updated, err
}

func (s *ImportService) generateItems(ctx context.Context, chunk string) ([]Item, error) {
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是题库生成助手。只输出 JSON。"},
		{Role: "user", Content: "从下面的工程实践文档切片中生成 3-5 道后端面试题，字段为 items 数组，每项包含 id, content, tags, skill_category, difficulty, expected_points, rubric, sample_answer, follow_up_hints。\n\n" + chunk},
	}, llm.Options{MaxTokens: 1600, Temperature: 0.2}, validateItemsJSON, 1)
	if err != nil {
		return nil, err
	}
	return parseQuestionBankItems("generated.json", []byte(resp.Content))
}

func (s *ImportService) enrichLocalItems(ctx context.Context, items []Item) ([]Item, []map[string]string, error) {
	provenances := make([]map[string]string, len(items))
	if s == nil || s.model == nil || len(items) == 0 {
		return items, provenances, nil
	}

	need := make([]Item, 0, len(items))
	for _, item := range items {
		if needsEnrichment(item) {
			need = append(need, item)
		}
	}
	if len(need) == 0 {
		return items, provenances, nil
	}

	enriched := make([]Item, 0, len(need))
	for start := 0; start < len(need); start += localEnrichmentBatchSize {
		end := start + localEnrichmentBatchSize
		if end > len(need) {
			end = len(need)
		}
		batch, err := s.enrichLocalBatch(ctx, need[start:end])
		if err != nil {
			return nil, nil, err
		}
		enriched = append(enriched, batch...)
	}
	byID := make(map[string]Item, len(enriched))
	byContent := make(map[string]Item, len(enriched))
	for _, item := range enriched {
		if id := strings.TrimSpace(item.ID); id != "" {
			byID[id] = item
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			byContent[content] = item
		}
	}

	out := make([]Item, 0, len(items))
	for _, item := range items {
		if !needsEnrichment(item) {
			out = append(out, item)
			continue
		}
		enrichedItem, ok := byID[strings.TrimSpace(item.ID)]
		if !ok {
			enrichedItem, ok = byContent[strings.TrimSpace(item.Content)]
		}
		if !ok {
			return nil, nil, fmt.Errorf("llm enrichment missing item %q", itemIdentity(item))
		}
		var fieldProvenance map[string]string
		item, fieldProvenance = mergeEnrichedItemWithProvenance(item, enrichedItem)
		provenances[len(out)] = fieldProvenance
		out = append(out, item)
	}
	return out, provenances, nil
}

func (s *ImportService) enrichLocalBatch(ctx context.Context, items []Item) ([]Item, error) {
	raw, err := json.Marshal(struct {
		Items []Item `json:"items"`
	}{Items: items})
	if err != nil {
		return nil, err
	}
	resp, err := llm.CallWithSchema(ctx, s.model, []llm.Message{
		{Role: "system", Content: "你是题库元数据补全助手。只输出 JSON。"},
		{Role: "user", Content: "补齐题库元数据。保留每道题的 id 和 content，返回 items 数组；每项补齐 tags, skill_category, difficulty, expected_points, rubric, sample_answer, follow_up_hints。必须为输入中的每一道题返回一项，不能新增题目，不能漏题。\n\n" + string(raw)},
	}, llm.Options{MaxTokens: 1800, Temperature: 0.2}, validateItemsJSON, 1)
	if err != nil {
		return nil, err
	}
	enriched, err := parseQuestionBankItems("enriched.json", []byte(resp.Content))
	if err != nil {
		return nil, err
	}
	if err := validateEnrichmentCoverage(items, enriched); err != nil {
		return nil, err
	}
	return enriched, nil
}

func parseQuestionBankItems(filename string, raw []byte) ([]Item, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".csv":
		return parseCSVItems(raw)
	case ".md", ".markdown":
		return parseMarkdownItems(raw), nil
	default:
		return parseJSONItems(raw)
	}
}

func parseJSONItems(raw []byte) ([]Item, error) {
	var items []Item
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	var wrapped struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("parse question bank json: %w", err)
	}
	return wrapped.Items, nil
}

func parseCSVItems(raw []byte) ([]Item, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse question bank csv: %w", err)
	}
	if len(records) < 2 {
		return nil, errors.New("question bank csv requires a header and at least one row")
	}
	header := map[string]int{}
	for i, col := range records[0] {
		header[strings.TrimSpace(col)] = i
	}
	var items []Item
	for _, row := range records[1:] {
		get := func(name string) string {
			if i, ok := header[name]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		difficulty, _ := strconv.Atoi(get("difficulty"))
		items = append(items, Item{
			ID:             get("id"),
			Content:        get("content"),
			Tags:           splitImportList(get("tags")),
			SkillCategory:  get("skill_category"),
			Difficulty:     difficulty,
			ExpectedPoints: splitImportList(get("expected_points")),
			Source:         get("source"),
			Scenario:       get("scenario"),
			RoleTags:       splitImportList(get("role_tags")),
			SampleAnswer:   get("sample_answer"),
			FollowUpHints:  splitImportList(get("follow_up_hints")),
			Locale:         get("locale"),
			Status:         get("status"),
		})
	}
	return items, nil
}

func parseMarkdownItems(raw []byte) []Item {
	blocks := splitMarkdownQuestionBlocks(string(raw))
	items := make([]Item, 0, len(blocks))
	for i, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		title := strings.Trim(strings.TrimSpace(lines[0]), "# ")
		content := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		if content == "" {
			content = title
		}
		items = append(items, Item{
			ID:         importGeneratedID("md", fmt.Sprintf("%d:%s", i, title)),
			Content:    content,
			Tags:       []string{"imported"},
			Difficulty: 3,
			Source:     "import:markdown",
		})
	}
	return items
}

func splitMarkdownQuestionBlocks(raw string) []string {
	var blocks []string
	var current strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "##") && current.Len() > 0 {
			blocks = append(blocks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}
	return blocks
}

func validateItemsJSON(raw []byte) error {
	if err := llm.ValidateJSON(raw); err != nil {
		return err
	}
	items, err := parseJSONItems(raw)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return errors.New("items must not be empty")
	}
	return nil
}

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

func needsEnrichment(item Item) bool {
	return strings.TrimSpace(item.SkillCategory) == "" ||
		strings.TrimSpace(item.SkillCategory) == "general" ||
		item.Difficulty == 0 ||
		len(item.Tags) == 0 ||
		len(item.ExpectedPoints) == 0 ||
		len(item.Rubric) == 0 ||
		strings.TrimSpace(item.SampleAnswer) == "" ||
		len(item.FollowUpHints) == 0
}

func mergeEnrichedItem(base, enriched Item) Item {
	merged, _ := mergeEnrichedItemWithProvenance(base, enriched)
	return merged
}

func mergeEnrichedItemWithProvenance(base, enriched Item) (Item, map[string]string) {
	provenance := map[string]string{}
	if strings.TrimSpace(base.SkillCategory) == "" || strings.TrimSpace(base.SkillCategory) == "general" {
		if strings.TrimSpace(enriched.SkillCategory) != "" {
			if strings.TrimSpace(base.SkillCategory) == "general" {
				provenance["skill_category"] = "merged"
			} else {
				provenance["skill_category"] = "llm"
			}
		}
		base.SkillCategory = enriched.SkillCategory
	}
	if base.Difficulty == 0 {
		if enriched.Difficulty != 0 {
			provenance["difficulty"] = "llm"
		}
		base.Difficulty = enriched.Difficulty
	}
	if len(base.Tags) == 0 {
		if len(enriched.Tags) > 0 {
			provenance["tags"] = "llm"
		}
		base.Tags = enriched.Tags
	}
	if len(base.ExpectedPoints) == 0 {
		if len(enriched.ExpectedPoints) > 0 {
			provenance["expected_points"] = "llm"
		}
		base.ExpectedPoints = enriched.ExpectedPoints
	}
	if len(base.Rubric) == 0 {
		if len(enriched.Rubric) > 0 {
			provenance["rubric"] = "llm"
		}
		base.Rubric = enriched.Rubric
	}
	if strings.TrimSpace(base.SampleAnswer) == "" {
		if strings.TrimSpace(enriched.SampleAnswer) != "" {
			provenance["sample_answer"] = "llm"
		}
		base.SampleAnswer = enriched.SampleAnswer
	}
	if len(base.FollowUpHints) == 0 {
		if len(enriched.FollowUpHints) > 0 {
			provenance["follow_up_hints"] = "llm"
		}
		base.FollowUpHints = enriched.FollowUpHints
	}
	return base, provenance
}

func importFieldProvenance(parsed Item, normalized Item, original *Item) map[string]string {
	provenance := map[string]string{}
	mark := func(field string, uploaded bool, defaulted bool) {
		switch {
		case uploaded:
			provenance[field] = "uploaded"
		case defaulted:
			provenance[field] = "default"
		}
	}
	if original == nil {
		mark("skill_category", strings.TrimSpace(parsed.SkillCategory) != "", strings.TrimSpace(normalized.SkillCategory) == "general")
		mark("difficulty", parsed.Difficulty != 0, normalized.Difficulty == 3)
		mark("tags", len(parsed.Tags) > 0, false)
		mark("expected_points", len(parsed.ExpectedPoints) > 0, false)
		mark("rubric", len(parsed.Rubric) > 0, false)
		mark("sample_answer", strings.TrimSpace(parsed.SampleAnswer) != "", false)
		mark("follow_up_hints", len(parsed.FollowUpHints) > 0, false)
		for field, source := range provenance {
			if source == "uploaded" {
				provenance[field] = "generated"
			}
		}
		return provenance
	}
	mark("skill_category", strings.TrimSpace(original.SkillCategory) != "", strings.TrimSpace(normalized.SkillCategory) == "general")
	mark("difficulty", original.Difficulty != 0, normalized.Difficulty == 3)
	mark("tags", len(original.Tags) > 0, false)
	mark("expected_points", len(original.ExpectedPoints) > 0, false)
	mark("rubric", len(original.Rubric) > 0, false)
	mark("sample_answer", strings.TrimSpace(original.SampleAnswer) != "", false)
	mark("follow_up_hints", len(original.FollowUpHints) > 0, false)
	return provenance
}

func validateEnrichmentCoverage(inputs, enriched []Item) error {
	byID := make(map[string]struct{}, len(enriched))
	byContent := make(map[string]struct{}, len(enriched))
	for _, item := range enriched {
		if id := strings.TrimSpace(item.ID); id != "" {
			byID[id] = struct{}{}
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			byContent[content] = struct{}{}
		}
	}
	for _, item := range inputs {
		if id := strings.TrimSpace(item.ID); id != "" {
			if _, ok := byID[id]; ok {
				continue
			}
		}
		if content := strings.TrimSpace(item.Content); content != "" {
			if _, ok := byContent[content]; ok {
				continue
			}
		}
		return fmt.Errorf("llm enrichment missing item %q", itemIdentity(item))
	}
	return nil
}

func itemIdentity(item Item) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return id
	}
	return strings.TrimSpace(item.Content)
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

func splitImportList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '，' || r == '、'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func importItemAccepted(item ImportItem) bool {
	return item.ReviewStatus == "" || item.ReviewStatus == ImportReviewStatusAccepted
}

func normalizedImportReviewStatus(status string) string {
	if status == ImportReviewStatusRejected {
		return ImportReviewStatusRejected
	}
	return ImportReviewStatusAccepted
}

func importItemHasCompleteReviewFields(item Item) bool {
	return strings.TrimSpace(item.Content) != "" &&
		strings.TrimSpace(item.SkillCategory) != "" &&
		item.Difficulty > 0 &&
		len(item.Tags) > 0 &&
		len(item.ExpectedPoints) > 0 &&
		len(item.Rubric) > 0 &&
		strings.TrimSpace(item.SampleAnswer) != "" &&
		len(item.FollowUpHints) > 0
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

type LocalImportSpool struct {
	Root string
}

func NewLocalImportSpool(root string) *LocalImportSpool {
	return &LocalImportSpool{Root: root}
}

func (s *LocalImportSpool) Save(ctx context.Context, jobID string, file ImportFile) (ImportFileRef, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return ImportFileRef{}, errors.New("import spool root not configured")
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return ImportFileRef{}, fmt.Errorf("resolve import spool root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ImportFileRef{}, fmt.Errorf("create import spool root: %w", err)
	}
	tmp, err := os.CreateTemp(root, "upload-*.tmp")
	if err != nil {
		return ImportFileRef{}, fmt.Errorf("create import spool temp file: %w", err)
	}
	tmpPath := tmp.Name()
	var copied int64
	copyErr := error(nil)
	done := make(chan struct{})
	go func() {
		copied, copyErr = io.Copy(tmp, file.Reader)
		close(done)
	}()
	select {
	case <-ctx.Done():
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return ImportFileRef{}, ctx.Err()
	case <-done:
	}
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return ImportFileRef{}, fmt.Errorf("write import spool file: %w", copyErr)
	}
	finalPath, err := s.pathFor(root, jobID)
	if err != nil {
		_ = os.Remove(tmpPath)
		return ImportFileRef{}, err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return ImportFileRef{}, fmt.Errorf("commit import spool file: %w", err)
	}
	return ImportFileRef{
		Path:        finalPath,
		Filename:    file.Filename,
		ContentType: file.ContentType,
		Size:        copied,
	}, nil
}

func (s *LocalImportSpool) Open(ctx context.Context, ref ImportFileRef) (ImportFile, func(), error) {
	if err := ctx.Err(); err != nil {
		return ImportFile{}, func() {}, err
	}
	path, err := s.safePath(ref.Path)
	if err != nil {
		return ImportFile{}, func() {}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return ImportFile{}, func() {}, fmt.Errorf("open import spool file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return ImportFile{}, func() {}, fmt.Errorf("stat import spool file: %w", err)
	}
	return ImportFile{
		Filename:    ref.Filename,
		ContentType: ref.ContentType,
		Reader:      f,
		Size:        info.Size(),
	}, func() { _ = f.Close() }, nil
}

func (s *LocalImportSpool) Delete(ctx context.Context, ref ImportFileRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safePath(ref.Path)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete import spool file: %w", err)
	}
	return nil
}

func (s *LocalImportSpool) pathFor(root, jobID string) (string, error) {
	if strings.TrimSpace(jobID) == "" || strings.ContainsAny(jobID, `\/:`) {
		return "", errors.New("invalid import job id for spool path")
	}
	return filepath.Join(root, jobID+".upload"), nil
}

func (s *LocalImportSpool) safePath(raw string) (string, error) {
	if s == nil || strings.TrimSpace(s.Root) == "" {
		return "", errors.New("import spool root not configured")
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", fmt.Errorf("resolve import spool root: %w", err)
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve import spool path: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("check import spool path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("import spool path escapes root")
	}
	return path, nil
}

func importGeneratedID(prefix, s string) string {
	sum := sha1.Sum([]byte(s))
	return prefix + "-" + hex.EncodeToString(sum[:])[:12]
}

type MemoryImportStore struct {
	mu     sync.Mutex
	jobs   map[string]ImportJob
	chunks map[string][]ImportChunk
	items  map[string][]ImportItem
}

func NewMemoryImportStore() *MemoryImportStore {
	return &MemoryImportStore{
		jobs:   map[string]ImportJob{},
		chunks: map[string][]ImportChunk{},
		items:  map[string][]ImportItem{},
	}
}

func (s *MemoryImportStore) CreateJob(_ context.Context, job ImportJob) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) UpdateJob(_ context.Context, job ImportJob) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return ImportJob{}, ErrImportNotFound
	}
	job.UpdatedAt = time.Now().UTC()
	s.jobs[job.ID] = job
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) GetJob(_ context.Context, id string) (ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return ImportJob{}, ErrImportNotFound
	}
	return cloneImportJob(job), nil
}

func (s *MemoryImportStore) ListJobs(_ context.Context) ([]ImportJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ImportJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, cloneImportJob(job))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryImportStore) AddChunks(_ context.Context, chunks []ImportChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chunk := range chunks {
		s.chunks[chunk.JobID] = append(s.chunks[chunk.JobID], cloneImportChunk(chunk))
	}
	return nil
}

func (s *MemoryImportStore) ListChunks(_ context.Context, jobID string) ([]ImportChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]ImportChunk(nil), s.chunks[jobID]...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (s *MemoryImportStore) AddItems(_ context.Context, items []ImportItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.items[item.JobID] = append(s.items[item.JobID], cloneImportItem(item))
	}
	return nil
}

func (s *MemoryImportStore) ListItems(_ context.Context, jobID string) ([]ImportItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]ImportItem(nil), s.items[jobID]...)
	for i := range out {
		out[i] = cloneImportItem(out[i])
	}
	return out, nil
}

func (s *MemoryImportStore) UpdateItems(_ context.Context, items []ImportItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		for jobID := range s.items {
			for i := range s.items[jobID] {
				if s.items[jobID][i].ID == item.ID {
					s.items[jobID][i] = cloneImportItem(item)
				}
			}
		}
	}
	return nil
}

func (s *MemoryImportStore) UpdateItemReviews(_ context.Context, jobID string, itemIDs []string, reviewStatus string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := map[string]struct{}{}
	for _, id := range itemIDs {
		wanted[id] = struct{}{}
	}
	for i := range s.items[jobID] {
		if _, ok := wanted[s.items[jobID][i].ID]; !ok {
			continue
		}
		if s.items[jobID][i].Status != ImportItemStatusValid {
			continue
		}
		s.items[jobID][i].ReviewStatus = reviewStatus
		s.items[jobID][i].UpdatedAt = time.Now().UTC()
	}
	return nil
}

// 重置作业数据，删除所有与作业相关的切片和题目，但保留作业本身。这在重新处理同一文件时很有用，可以避免重复创建作业并保留作业的元数据和状态。
func (s *MemoryImportStore) ResetJobData(_ context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chunks, jobID)
	delete(s.items, jobID)
	return nil
}

// 检查作业是否存在，是否被其他人锁定，如果未锁定或已被当前用户锁定，则锁定作业并返回 true；如果被其他人锁定，则返回 false。
func (s *MemoryImportStore) TryAcquireJob(_ context.Context, jobID, ownerID string, leaseFor time.Duration) (ImportJob, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return ImportJob{}, false, ErrImportNotFound
	}
	now := time.Now().UTC()
	if job.OwnerID != "" && job.OwnerID != ownerID && job.LeaseUntil.After(now) {
		return cloneImportJob(job), false, nil
	}
	if leaseFor <= 0 {
		leaseFor = 2 * time.Minute
	}
	job.OwnerID = ownerID
	job.LeaseUntil = now.Add(leaseFor)
	job.UpdatedAt = now
	s.jobs[job.ID] = job
	return cloneImportJob(job), true, nil
}

func cloneImportJob(job ImportJob) ImportJob {
	job.Metadata = cloneStringMap(job.Metadata)
	return job
}

func cloneImportChunk(chunk ImportChunk) ImportChunk {
	chunk.Metadata = cloneStringMap(chunk.Metadata)
	return chunk
}

func cloneImportItem(item ImportItem) ImportItem {
	item.Item = cloneItem(item.Item)
	if item.OriginalItem != nil {
		original := cloneItem(*item.OriginalItem)
		item.OriginalItem = &original
	}
	item.FieldProvenance = cloneStringMap(item.FieldProvenance)
	item.Errors = append([]string(nil), item.Errors...)
	return item
}

func cloneImportItems(items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, cloneItem(item))
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
