package questionbank

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"interview-agent/internal/embedding"
	"interview-agent/internal/llm"
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
	if item.SkillCategory == "" || item.SkillCategory == "general" {
		t.Fatalf("SkillCategory = %q, want LLM-enriched category", item.SkillCategory)
	}
	if len(item.Tags) == 0 || len(item.ExpectedPoints) == 0 || len(item.Rubric) == 0 || len(item.FollowUpHints) == 0 || item.SampleAnswer == "" {
		t.Fatalf("item was not enriched enough: %+v", item)
	}
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
