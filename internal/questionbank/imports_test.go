package questionbank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"interview-agent/internal/embedding"
	"interview-agent/internal/llm"
	"interview-agent/internal/parser"
)

func TestLocalImportSpool_SaveOpenDelete(t *testing.T) {
	ctx := context.Background()
	spool := NewLocalImportSpool(t.TempDir())
	ref, err := spool.Save(ctx, "job-001", ImportFile{
		Filename:    "questions.json",
		ContentType: "application/json",
		Reader:      bytes.NewBufferString(`[{"id":"spooled"}]`),
		Size:        18,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if ref.Path == "" || ref.Size <= 0 {
		t.Fatalf("ref = %+v", ref)
	}

	file, closeFn, err := spool.Open(ctx, ref)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	raw, err := io.ReadAll(file.Reader)
	closeFn()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(raw) != `[{"id":"spooled"}]` {
		t.Fatalf("raw = %q", raw)
	}
	if file.Filename != "questions.json" || file.ContentType != "application/json" {
		t.Fatalf("file metadata = %+v", file)
	}
	if err := spool.Delete(ctx, ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := spool.Open(ctx, ref); err == nil {
		t.Fatal("Open after Delete should fail")
	}
}

func TestImportService_ImportLocalJSONStagesItemsBeforeCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{
				"id":"go-import-001",
				"content":"Go channel 关闭后读写分别会发生什么？",
				"tags":["go","channel"],
				"skill_category":"go",
				"difficulty":3,
				"expected_points":["读到零值","重复 close panic"]
			}
		]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if job.Status != ImportStatusReady || job.ValidItems != 1 || job.TotalItems != 1 {
		t.Fatalf("job = %+v, want ready with one valid item", job)
	}
	if _, err := store.Get(ctx, "go-import-001"); err == nil {
		t.Fatal("import should stage items, not write question_bank before commit")
	}

	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 || items[0].Status != ImportItemStatusValid {
		t.Fatalf("items = %+v, want one valid staged item", items)
	}
	if items[0].ReviewStatus != ImportReviewStatusAccepted {
		t.Fatalf("review_status = %q, want accepted by default", items[0].ReviewStatus)
	}
	assertFieldProvenance(t, items[0], map[string]string{
		"skill_category":  "uploaded",
		"difficulty":      "uploaded",
		"tags":            "uploaded",
		"expected_points": "uploaded",
	})
}

func TestImportService_CommitSkipsRejectedItems(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"go-accept-001","content":"Go channel close 行为？","skill_category":"go","difficulty":3},
			{"id":"go-reject-001","content":"Go map 并发读写？","skill_category":"go","difficulty":3}
		]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want two staged items", items)
	}

	if _, _, err := service.ReviewItems(ctx, job.ID, []string{job.ID + ":go-reject-001"}, ImportReviewStatusRejected); err != nil {
		t.Fatalf("ReviewItems: %v", err)
	}
	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 1 {
		t.Fatalf("ImportedItems = %d, want 1", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, "go-accept-001"); err != nil {
		t.Fatalf("accepted item should be committed: %v", err)
	}
	if _, err := store.Get(ctx, "go-reject-001"); err == nil {
		t.Fatal("rejected item should not be committed")
	}
}

func TestCommitSkipsAgentRejectedItems(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: strings.NewReader(`[{
			"id":"reject-agent-001",
			"content":"Go channel 关闭后接收行为是什么？",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag","panic on send"]
		}]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one item", items)
	}
	items[0].AgentReviewStatus = ImportAgentReviewRejected
	items[0].AgentReviewReason = "not grounded in source"
	if err := imports.UpdateItems(ctx, items); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("ImportedItems = %d, want 0", committed.ImportedItems)
	}
	committedItems, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems after commit: %v", err)
	}
	for _, item := range committedItems {
		if item.QuestionID == "reject-agent-001" && item.Status == ImportItemStatusImported {
			t.Fatalf("agent rejected item was imported: %+v", item)
		}
	}
	if _, err := store.Get(ctx, "reject-agent-001"); err == nil {
		t.Fatal("agent rejected item should not be written to question bank")
	}
}

func TestImportService_ImportLocalQuestionOnlyUsesLLMEnrichment(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
		Model:   llm.NewMockChatModel(""),
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"question-only-001","content":"Go map 并发读写为什么会 panic？"}
		]`),
		Size: 128,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if job.Status != ImportStatusReady || job.ValidItems != 1 {
		t.Fatalf("job = %+v, want ready with one valid item", job)
	}

	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one staged item", items)
	}
	item := items[0].Item
	if item.ID != "question-only-001" || item.Content != "Go map 并发读写为什么会 panic？" {
		t.Fatalf("enrichment should preserve id/content: %+v", item)
	}
	if items[0].OriginalItem == nil {
		t.Fatal("staged enriched import should keep original item for review")
	}
	if items[0].OriginalItem.SkillCategory != "" || len(items[0].OriginalItem.Tags) != 0 {
		t.Fatalf("original item should remain raw upload metadata: %+v", items[0].OriginalItem)
	}
	if item.SkillCategory == "" || item.SkillCategory == "general" {
		t.Fatalf("SkillCategory = %q, want LLM-enriched category", item.SkillCategory)
	}
	if len(item.Tags) == 0 || len(item.ExpectedPoints) == 0 || len(item.Rubric) == 0 || len(item.FollowUpHints) == 0 || item.SampleAnswer == "" {
		t.Fatalf("item was not enriched enough: %+v", item)
	}
	assertFieldProvenance(t, items[0], map[string]string{
		"skill_category":  "llm",
		"difficulty":      "llm",
		"tags":            "llm",
		"expected_points": "llm",
		"rubric":          "llm",
		"sample_answer":   "llm",
		"follow_up_hints": "llm",
	})
}

func TestImportService_ImportLocalQuestionOnlyWithoutLLMFallsBackToDefaults(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"question-only-002","content":"Redis 缓存击穿怎么处理？"}
		]`),
		Size: 128,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one staged item", items)
	}
	item := items[0].Item
	if item.SkillCategory != "general" || item.Difficulty != 3 {
		t.Fatalf("fallback item = %+v, want old default metadata", item)
	}
	if len(item.ExpectedPoints) != 0 || len(item.Rubric) != 0 || len(item.FollowUpHints) != 0 {
		t.Fatalf("fallback should not invent rich metadata without LLM: %+v", item)
	}
	assertFieldProvenance(t, items[0], map[string]string{
		"skill_category": "default",
		"difficulty":     "default",
	})
}

func TestImportService_StageGeneratedItemFieldProvenance(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "source.md"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	job, err = service.stageItems(ctx, job, "chunk-001", []Item{{
		ID:             "generated-001",
		Content:        "如何治理 Redis 热 key？",
		ExpectedPoints: []string{"发现热 key", "缓存隔离"},
	}})
	if err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	if job.ValidItems != 1 {
		t.Fatalf("ValidItems = %d, want 1", job.ValidItems)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one item", items)
	}
	if items[0].OriginalItem != nil {
		t.Fatalf("generated import item should not invent original item: %+v", items[0].OriginalItem)
	}
	assertFieldProvenance(t, items[0], map[string]string{
		"skill_category":  "default",
		"difficulty":      "default",
		"expected_points": "generated",
	})
}

func TestStageItemsPreservesAgentReviewAndSourceProvenance(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "go-runtime.md"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	job, err = service.stageItemsWithOriginalsAndProvenance(ctx, job, "chunk-001", []Item{{
		ID:             "go-runtime-001",
		Content:        "Go GMP 调度中 P 的作用是什么？",
		SkillCategory:  "go",
		Difficulty:     3,
		Tags:           []string{"go_concurrency", "scheduler"},
		ExpectedPoints: []string{"P 持有本地队列", "work stealing"},
	}}, nil, []map[string]string{{
		"source_type": "document",
		"source_hash": "sha256:abc",
	}})
	if err != nil {
		t.Fatalf("stageItemsWithOriginalsAndProvenance: %v", err)
	}
	if job.ValidItems != 1 {
		t.Fatalf("ValidItems = %d, want 1", job.ValidItems)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one item", items)
	}
	if items[0].AgentReviewStatus != ImportAgentReviewNeedsHumanReview {
		t.Fatalf("AgentReviewStatus = %q, want %q", items[0].AgentReviewStatus, ImportAgentReviewNeedsHumanReview)
	}
	if items[0].SourceProvenance["source_hash"] != "sha256:abc" {
		t.Fatalf("SourceProvenance = %+v, want source_hash", items[0].SourceProvenance)
	}

	cloned := cloneImportItem(items[0])
	cloned.SourceProvenance["source_hash"] = "changed"
	if items[0].SourceProvenance["source_hash"] != "sha256:abc" {
		t.Fatalf("clone mutated original provenance: %+v", items[0].SourceProvenance)
	}
}

func TestImportDocumentStagesSourceProvenance(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
		Parser:  &parser.MockParser{Text: "Go 服务需要 context 超时、重试和熔断。", PageCount: 1},
		Model:   llm.NewMockChatModel(""),
	})

	job, err := service.ImportDocument(ctx, ImportFile{
		Filename:    "go-resilience.md",
		ContentType: "text/markdown",
		Reader:      strings.NewReader("Go 服务需要 context 超时、重试和熔断。"),
		Size:        64,
	})
	if err != nil {
		t.Fatalf("ImportDocument: %v", err)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one generated item", items)
	}
	if items[0].AgentReviewStatus != ImportAgentReviewNeedsHumanReview {
		t.Fatalf("AgentReviewStatus = %q, want %q", items[0].AgentReviewStatus, ImportAgentReviewNeedsHumanReview)
	}
	if got := items[0].SourceProvenance["source_type"]; got != ImportSourceDocument {
		t.Fatalf("source_type = %q, want %q; provenance=%+v", got, ImportSourceDocument, items[0].SourceProvenance)
	}
	if got := items[0].SourceProvenance["filename"]; got != "go-resilience.md" {
		t.Fatalf("filename = %q, want go-resilience.md; provenance=%+v", got, items[0].SourceProvenance)
	}
	if !strings.HasPrefix(items[0].SourceProvenance["source_hash"], "sha256:") {
		t.Fatalf("source_hash missing: %+v", items[0].SourceProvenance)
	}
	if !strings.HasPrefix(items[0].SourceProvenance["chunk_hash"], "sha256:") {
		t.Fatalf("chunk_hash missing: %+v", items[0].SourceProvenance)
	}
}

func TestPGImportMetadataPackingRoundTripsAgentReviewAndSource(t *testing.T) {
	item := ImportItem{
		FieldProvenance: map[string]string{"difficulty": "generated"},
		AgentReviewStatus: ImportAgentReviewAutoApproved,
		AgentReviewReason: "complete and grounded",
		SourceProvenance: map[string]string{
			"source_type": ImportSourceDocument,
			"source_hash": "sha256:abc",
		},
	}

	packed := packImportItemMetadata(item)
	roundTrip := ImportItem{FieldProvenance: cloneStringMap(packed)}
	unpackImportItemMetadata(&roundTrip)

	if roundTrip.FieldProvenance["difficulty"] != "generated" {
		t.Fatalf("FieldProvenance = %+v, want difficulty", roundTrip.FieldProvenance)
	}
	if roundTrip.AgentReviewStatus != ImportAgentReviewAutoApproved {
		t.Fatalf("AgentReviewStatus = %q", roundTrip.AgentReviewStatus)
	}
	if roundTrip.AgentReviewReason != "complete and grounded" {
		t.Fatalf("AgentReviewReason = %q", roundTrip.AgentReviewReason)
	}
	if roundTrip.SourceProvenance["source_hash"] != "sha256:abc" {
		t.Fatalf("SourceProvenance = %+v", roundTrip.SourceProvenance)
	}
	if _, ok := roundTrip.FieldProvenance[importMetaAgentReviewStatus]; ok {
		t.Fatalf("reserved key leaked into field provenance: %+v", roundTrip.FieldProvenance)
	}
}

func TestPGImportStoreJSONMetadataEncodingUsesObjects(t *testing.T) {
	if got := string(marshalOriginalItemJSON(nil)); got != "{}" {
		t.Fatalf("nil original raw json = %s, want {}", got)
	}
	if got := string(marshalStringMapJSON(nil)); got != "{}" {
		t.Fatalf("nil field provenance json = %s, want {}", got)
	}
}

func assertFieldProvenance(t *testing.T, item ImportItem, want map[string]string) {
	t.Helper()
	for field, source := range want {
		if got := item.FieldProvenance[field]; got != source {
			t.Fatalf("FieldProvenance[%q] = %q, want %q; full provenance = %+v", field, got, source, item.FieldProvenance)
		}
	}
}

func TestImportService_ImportLocalEnrichmentBatchesRequests(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	model := &recordingEnrichmentModel{}
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
		Model:   model,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"q1","content":"Go channel 如何避免 goroutine 泄漏？"},
			{"id":"q2","content":"Redis 热 key 如何治理？"},
			{"id":"q3","content":"PostgreSQL 慢查询如何排查？"},
			{"id":"q4","content":"服务雪崩如何用熔断治理？"}
		]`),
		Size: 512,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if job.Status != ImportStatusReady || job.ValidItems != 4 {
		t.Fatalf("job = %+v, want ready with four valid items", job)
	}
	if model.calls != 2 {
		t.Fatalf("LLM calls = %d, want 2 batches", model.calls)
	}
}

func TestImportService_ImportLocalEnrichmentFailsOnMissingReturnedItem(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
		Model:   missingEnrichmentModel{},
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"q1","content":"Go channel 如何避免 goroutine 泄漏？"},
			{"id":"q2","content":"Redis 热 key 如何治理？"}
		]`),
		Size: 256,
	})
	if err == nil {
		t.Fatal("ImportLocalQuestionBank should fail when LLM omits an item")
	}
	if job.Status != ImportStatusFailed {
		t.Fatalf("job status = %q, want failed", job.Status)
	}
}

func TestImportService_EnqueueImportRunsInBackground(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports:  imports,
		Writer:   store,
		Embedder: embedding.NewMockEmbedder(8),
		Spool:    NewLocalImportSpool(t.TempDir()),
	})

	job, err := service.EnqueueImport(ctx, ImportSourceQuestionBank, ImportFile{
		Filename: "async.json",
		Reader: bytes.NewBufferString(`[
			{"id":"async-import-001","content":"Redis 缓存雪崩如何治理？","skill_category":"redis","difficulty":3}
		]`),
		Size: 128,
	})
	if err != nil {
		t.Fatalf("EnqueueImport: %v", err)
	}
	if job.Status != ImportStatusQueued {
		t.Fatalf("initial status = %q, want queued", job.Status)
	}
	if job.Metadata["spool_path"] == "" {
		t.Fatalf("queued job should record spool_path: %+v", job.Metadata)
	}
	waitFor(t, time.Second, func() bool {
		got, _, err := service.Get(ctx, job.ID)
		return err == nil && got.Status == ImportStatusReady && got.ValidItems == 1
	})
	waitFor(t, time.Second, func() bool {
		_, closeFn, err := service.spool.Open(ctx, ImportFileRef{
			Path:        job.Metadata["spool_path"],
			Filename:    job.Filename,
			ContentType: job.Metadata["content_type"],
		})
		if err == nil {
			closeFn()
		}
		return err != nil
	})
	if _, err := store.Get(ctx, "async-import-001"); err == nil {
		t.Fatal("async import should stage only; commit remains explicit")
	}

	if _, err := service.EnqueueCommit(ctx, job.ID); err != nil {
		t.Fatalf("EnqueueCommit: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		got, _, err := service.Get(ctx, job.ID)
		return err == nil && got.Status == ImportStatusCommitted && got.ImportedItems == 1
	})
	if _, err := store.Get(ctx, "async-import-001"); err != nil {
		t.Fatalf("async commit should import item: %v", err)
	}
}

func TestImportService_RecoverPendingQueuedImport(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	spool := NewLocalImportSpool(t.TempDir())
	seed := NewImportService(ImportServiceDeps{Imports: imports, Writer: store, Spool: spool})

	job := newImportJob(ImportSourceQuestionBank, "recover.json")
	created, err := imports.CreateJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := spool.Save(ctx, created.ID, ImportFile{
		Filename: "recover.json",
		Reader: bytes.NewBufferString(`[
			{"id":"recover-import-001","content":"Go pprof 如何定位 CPU 热点？","skill_category":"go","difficulty":4}
		]`),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	created.Status = ImportStatusQueued
	created.Metadata["spool_path"] = ref.Path
	created.Metadata["size"] = "128"
	if _, err := imports.UpdateJob(ctx, created); err != nil {
		t.Fatal(err)
	}

	recovered, err := seed.RecoverPendingJobs(ctx)
	if err != nil {
		t.Fatalf("RecoverPendingJobs: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	waitFor(t, time.Second, func() bool {
		got, _, err := seed.Get(ctx, created.ID)
		return err == nil && got.Status == ImportStatusReady && got.ValidItems == 1
	})
	if _, err := store.Get(ctx, "recover-import-001"); err == nil {
		t.Fatal("recovered import should stage only")
	}
}

func TestImportService_RecoverPendingCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports:  imports,
		Writer:   store,
		Embedder: embedding.NewMockEmbedder(8),
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "recover-commit.json",
		Reader: bytes.NewBufferString(`[
			{"id":"recover-commit-001","content":"Redis 分布式锁有哪些坑？","skill_category":"redis","difficulty":4}
		]`),
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	job.Status = ImportStatusCommitting
	if _, err := imports.UpdateJob(ctx, job); err != nil {
		t.Fatal(err)
	}

	recovered, err := service.RecoverPendingJobs(ctx)
	if err != nil {
		t.Fatalf("RecoverPendingJobs: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	waitFor(t, time.Second, func() bool {
		got, _, err := service.Get(ctx, job.ID)
		return err == nil && got.Status == ImportStatusCommitted && got.ImportedItems == 1
	})
	if _, err := store.Get(ctx, "recover-commit-001"); err != nil {
		t.Fatalf("recovered commit should import item: %v", err)
	}
}

func TestImportService_RecoverPendingJobsUsesLease(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	spool := NewLocalImportSpool(t.TempDir())

	job := newImportJob(ImportSourceQuestionBank, "lease.json")
	created, err := imports.CreateJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := spool.Save(ctx, created.ID, ImportFile{
		Filename: "lease.json",
		Reader: bytes.NewBufferString(`[
			{"id":"lease-import-001","content":"Redis 慢日志如何分析？","skill_category":"redis","difficulty":3}
		]`),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	created.Status = ImportStatusQueued
	created.Metadata["spool_path"] = ref.Path
	created.Metadata["size"] = "128"
	if _, err := imports.UpdateJob(ctx, created); err != nil {
		t.Fatal(err)
	}

	first := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
		Spool:   spool,
		OwnerID: "worker-a",
	})
	second := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
		Spool:   spool,
		OwnerID: "worker-b",
	})

	n1, err := first.RecoverPendingJobs(ctx)
	if err != nil {
		t.Fatalf("first RecoverPendingJobs: %v", err)
	}
	n2, err := second.RecoverPendingJobs(ctx)
	if err != nil {
		t.Fatalf("second RecoverPendingJobs: %v", err)
	}
	if n1 != 1 || n2 != 0 {
		t.Fatalf("recover counts = %d/%d, want 1/0", n1, n2)
	}
	leased, err := imports.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if leased.OwnerID != "worker-a" || leased.LeaseUntil.IsZero() {
		t.Fatalf("job lease = owner %q until %v, want worker-a lease", leased.OwnerID, leased.LeaseUntil)
	}
}

func TestImportService_ShutdownWaitsForBackgroundTasks(t *testing.T) {
	service := NewImportService(ImportServiceDeps{Imports: NewMemoryImportStore()})
	started := make(chan struct{})
	release := make(chan struct{})

	if ok := service.runAsync(func() {
		close(started)
		<-release
	}); !ok {
		t.Fatal("runAsync should accept work before shutdown")
	}
	<-started

	done := make(chan error, 1)
	go func() {
		done <- service.Shutdown(context.Background())
	}()
	select {
	case err := <-done:
		t.Fatalf("Shutdown returned before background task finished: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not return after background task finished")
	}
}

func TestImportService_ShutdownRejectsNewBackgroundTasks(t *testing.T) {
	service := NewImportService(ImportServiceDeps{Imports: NewMemoryImportStore()})
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if ok := service.runAsync(func() {
		t.Fatal("background task should not run after shutdown")
	}); ok {
		t.Fatal("runAsync should reject work after shutdown")
	}
}

func TestImportService_EnqueueImportRejectsAfterShutdown(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  NewMemoryStore(nil),
		Spool:   NewLocalImportSpool(t.TempDir()),
	})
	if err := service.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	_, err := service.EnqueueImport(ctx, ImportSourceQuestionBank, ImportFile{
		Filename: "closed.json",
		Reader:   bytes.NewBufferString(`[]`),
	})
	if !errors.Is(err, ErrImportServiceShutdown) {
		t.Fatalf("EnqueueImport error = %v, want ErrImportServiceShutdown", err)
	}
	jobs, err := imports.ListJobs(ctx)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %+v, want none after rejected enqueue", jobs)
	}
}

func waitFor(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestImportService_CommitImportsOnlyValidItems(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{Imports: imports, Writer: store})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "mixed.json",
		Reader: bytes.NewBufferString(`[
			{"id":"valid-001","content":"Redis 热 key 如何发现和治理？","skill_category":"redis","difficulty":4},
			{"id":"invalid-001","content":"","skill_category":"go","difficulty":3}
		]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if job.ValidItems != 1 || job.InvalidItems != 1 {
		t.Fatalf("job = %+v, want one valid and one invalid item", job)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.Status != ImportStatusCommitted || committed.ImportedItems != 1 {
		t.Fatalf("committed job = %+v, want one imported item", committed)
	}
	if _, err := store.Get(ctx, "valid-001"); err != nil {
		t.Fatalf("valid item should be in question bank: %v", err)
	}
	if _, err := store.Get(ctx, "invalid-001"); err == nil {
		t.Fatal("invalid item should not be imported")
	}
}

func TestImportService_CommitEmbedsImportedItems(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports:  imports,
		Writer:   store,
		Embedder: embedding.NewMockEmbedder(8),
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: bytes.NewBufferString(`[
			{"id":"rag-ready-001","content":"PostgreSQL 慢查询应该如何定位？","skill_category":"postgresql","difficulty":4}
		]`),
		Size: 128,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if _, err := service.Commit(ctx, job.ID); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	embedding, model, ok := store.Embedding("rag-ready-001")
	if !ok {
		t.Fatal("committed item should have embedding")
	}
	if len(embedding) != 8 {
		t.Fatalf("embedding dim = %d, want 8", len(embedding))
	}
	if model != "mock" {
		t.Fatalf("embedding model = %q, want mock", model)
	}
}

type recordingEnrichmentModel struct {
	calls int
}

func (m *recordingEnrichmentModel) Name() string { return "recording-enrichment" }

func (m *recordingEnrichmentModel) Stream(ctx context.Context, messages []llm.Message, opts llm.Options) (<-chan llm.Chunk, error) {
	return nil, fmt.Errorf("stream not implemented")
}

func (m *recordingEnrichmentModel) Generate(ctx context.Context, messages []llm.Message, opts llm.Options) (*llm.Response, error) {
	m.calls++
	items := enrichmentRequestItems(messages)
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, enrichedTestItem(item))
	}
	raw, err := json.Marshal(struct {
		Items []Item `json:"items"`
	}{Items: out})
	if err != nil {
		return nil, err
	}
	return &llm.Response{Content: string(raw), Model: m.Name()}, nil
}

type missingEnrichmentModel struct{}

func (missingEnrichmentModel) Name() string { return "missing-enrichment" }

func (missingEnrichmentModel) Stream(ctx context.Context, messages []llm.Message, opts llm.Options) (<-chan llm.Chunk, error) {
	return nil, fmt.Errorf("stream not implemented")
}

func (missingEnrichmentModel) Generate(ctx context.Context, messages []llm.Message, opts llm.Options) (*llm.Response, error) {
	items := enrichmentRequestItems(messages)
	if len(items) > 1 {
		items = items[:1]
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, enrichedTestItem(item))
	}
	raw, err := json.Marshal(struct {
		Items []Item `json:"items"`
	}{Items: out})
	if err != nil {
		return nil, err
	}
	return &llm.Response{Content: string(raw), Model: "missing-enrichment"}, nil
}

func enrichmentRequestItems(messages []llm.Message) []Item {
	if len(messages) == 0 {
		return nil
	}
	content := messages[len(messages)-1].Content
	start := strings.Index(content, `{"items"`)
	if start < 0 {
		return nil
	}
	var request struct {
		Items []Item `json:"items"`
	}
	_ = json.Unmarshal([]byte(content[start:]), &request)
	return request.Items
}

func enrichedTestItem(item Item) Item {
	item.SkillCategory = "go"
	item.Difficulty = 3
	item.Tags = []string{"go", "backend"}
	item.ExpectedPoints = []string{"关键机制", "工程取舍"}
	item.Rubric = map[string]string{"good": "覆盖机制和落地方案"}
	item.SampleAnswer = "示例答案"
	item.FollowUpHints = []string{"追问一个边界场景"}
	return item
}
