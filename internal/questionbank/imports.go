package questionbank

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"interview-agent/internal/parser"
)

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
	items, packageMeta, err := parseQuestionBankImportPayload(file.Filename, raw)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	if meta := importJobMetadataForQuestionBankPackage(file, raw, packageMeta); len(meta) > 0 {
		if job.Metadata == nil {
			job.Metadata = map[string]string{}
		}
		for key, value := range meta {
			job.Metadata[key] = value
		}
		job, err = s.imports.UpdateJob(ctx, job)
		if err != nil {
			return s.failJob(ctx, job, err)
		}
	}
	originals := cloneImportItems(items)
	items, provenances, err := s.enrichLocalItems(ctx, items)
	if err != nil {
		return s.failJob(ctx, job, err)
	}
	sourceProvenance := sourceProvenanceForQuestionBankPackage(file, raw, packageMeta)
	for i := range provenances {
		if provenances[i] == nil {
			provenances[i] = map[string]string{}
		}
		for key, value := range sourceProvenance {
			provenances[i][key] = value
		}
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
		provenances := make([]map[string]string, len(items))
		for i := range provenances {
			provenances[i] = sourceProvenanceForChunk(file, raw, chunk)
		}
		job, err = s.stageItemsWithOriginalsAndProvenance(ctx, job, chunk.ID, items, nil, provenances)
		if err != nil {
			return job, err
		}
	}
	job.Status = ImportStatusReady
	return s.imports.UpdateJob(ctx, job)
}

func sourceProvenanceForChunk(file ImportFile, raw []byte, chunk ImportChunk) map[string]string {
	return map[string]string{
		"source_type":  ImportSourceDocument,
		"filename":     file.Filename,
		"content_type": file.ContentType,
		"source_hash":  "sha256:" + sha256Hex(raw),
		"chunk_id":     chunk.ID,
		"chunk_hash":   "sha256:" + sha256Hex([]byte(chunk.Content)),
	}
}

func importJobMetadataForQuestionBankPackage(file ImportFile, raw []byte, meta questionBankImportMetadata) map[string]string {
	out := sourceProvenanceForQuestionBankPackage(file, raw, meta)
	delete(out, "source_type")
	delete(out, "filename")
	delete(out, "content_type")
	delete(out, "source_hash")
	return out
}

func sourceProvenanceForQuestionBankPackage(file ImportFile, raw []byte, meta questionBankImportMetadata) map[string]string {
	out := map[string]string{
		"source_type": ImportSourceQuestionBank,
		"filename":    file.Filename,
		"source_hash": "sha256:" + sha256Hex(raw),
	}
	if strings.TrimSpace(file.ContentType) != "" {
		out["content_type"] = file.ContentType
	}
	if meta.SchemaVersion != "" {
		out["schema_version"] = meta.SchemaVersion
	}
	if meta.SourceRef != "" {
		out["source_ref"] = meta.SourceRef
	}
	if meta.ValidationReport != "" {
		out["validation_report"] = meta.ValidationReport
	}
	if meta.ReviewPolicy != "" {
		out["review_policy"] = meta.ReviewPolicy
	}
	return out
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
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

func (s *ImportService) Store() ImportStore {
	if s == nil {
		return nil
	}
	return s.imports
}
