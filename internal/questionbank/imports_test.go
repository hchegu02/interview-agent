package questionbank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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

func TestCommitSkipsAgentNeedsHumanReviewItems(t *testing.T) {
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
			"id":"review-agent-001",
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
	items[0].AgentReviewStatus = ImportAgentReviewNeedsHumanReview
	items[0].AgentReviewReason = "requires human confirmation"
	if err := imports.UpdateItems(ctx, items); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	_, err = service.Commit(ctx, job.ID)
	if err == nil {
		t.Fatal("Commit should require human review before committing")
	}
	if !strings.Contains(err.Error(), "requires human review") {
		t.Fatalf("Commit error = %v, want requires human review", err)
	}
	if _, err := store.Get(ctx, "review-agent-001"); err == nil {
		t.Fatal("needs_human_review item should not be written to question bank")
	}
	gotJob, err := imports.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if gotJob.Status != ImportStatusReady {
		t.Fatalf("job status = %s, want ready", gotJob.Status)
	}
}

func TestCommitAllowsAgentAutoApprovedItems(t *testing.T) {
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
			"id":"auto-agent-001",
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
	items[0].AgentReviewStatus = ImportAgentReviewAutoApproved
	items[0].AgentReviewReason = "complete and grounded"
	if err := imports.UpdateItems(ctx, items); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 1 {
		t.Fatalf("ImportedItems = %d, want 1", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, "auto-agent-001"); err != nil {
		t.Fatalf("auto_approved item should be written to question bank: %v", err)
	}
}

func TestImportCommitSkipsDuplicateContentInSameJob(t *testing.T) {
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
			"id":"dup-job-001",
			"content":"Go channel 关闭后接收行为是什么？",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag"]
		},{
			"id":"dup-job-002",
			"content":" go   channel 关闭后接收行为是什么？ ",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag"]
		}]`),
		Size: 512,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want two items", items)
	}
	for i := range items {
		items[i].AgentReviewStatus = ImportAgentReviewAutoApproved
		items[i].AgentReviewReason = "complete and grounded"
	}
	if err := imports.UpdateItems(ctx, items); err != nil {
		t.Fatalf("UpdateItems: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 1 {
		t.Fatalf("ImportedItems = %d, want 1", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, "dup-job-001"); err != nil {
		t.Fatalf("first duplicate should be written to question bank: %v", err)
	}
	if _, err := store.Get(ctx, "dup-job-002"); err == nil {
		t.Fatal("second duplicate should not be written to question bank")
	}
	committedItems, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems after commit: %v", err)
	}
	byQuestionID := map[string]ImportItem{}
	for _, item := range committedItems {
		byQuestionID[item.QuestionID] = item
	}
	if got := byQuestionID["dup-job-001"].Status; got != ImportItemStatusImported {
		t.Fatalf("first duplicate status = %q, want imported", got)
	}
	second := byQuestionID["dup-job-002"]
	if second.Status != ImportItemStatusValid {
		t.Fatalf("second duplicate status = %q, want valid", second.Status)
	}
	if second.AgentReviewStatus != ImportAgentReviewRejected || second.AgentReviewReason != qualityFlagDuplicateContent {
		t.Fatalf("second duplicate review = %q/%q, want rejected/%s", second.AgentReviewStatus, second.AgentReviewReason, qualityFlagDuplicateContent)
	}
}

func TestImportServiceCommitBlocksDirtyAcceptedQuestionContent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceQuestionBank, "dirty.json"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	dirty := completeImportTestItem("dirty-agent-001")
	dirty.Content = "有使用过吗你这个agent项目就是四个智能体 用langchain也可以实现啊 你用了langGraph 有哪些是用langchain不能实现的吗--（无法反驳.."
	if _, err := service.stageItems(ctx, job, "", []Item{dirty}); err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	markImportJobReady(t, ctx, imports, job.ID)
	if _, _, err := service.ReviewAllValidItems(ctx, job.ID, ImportReviewStatusAccepted, true); err != nil {
		t.Fatalf("ReviewAllValidItems: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("ImportedItems = %d, want 0", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, dirty.ID); err == nil {
		t.Fatalf("dirty question %s should not be committed", dirty.ID)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 || items[0].AgentReviewStatus != ImportAgentReviewRejected {
		t.Fatalf("staged item = %+v, want rejected", items)
	}
	if !strings.Contains(items[0].AgentReviewReason, QualityFlagDirtyNoteMarker) {
		t.Fatalf("AgentReviewReason = %q, want dirty flag", items[0].AgentReviewReason)
	}
}

func TestImportCommitSkipsDuplicateExistingActiveContent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore([]Item{{
		ID:            "existing-active-001",
		Content:       "Go channel 关闭后接收行为是什么？",
		SkillCategory: "go",
		Difficulty:    3,
	}})
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: strings.NewReader(`[{
			"id":"dup-existing-001",
			"content":" go   channel 关闭后接收行为是什么？ ",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag"]
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
	items[0].AgentReviewStatus = ImportAgentReviewAutoApproved
	items[0].AgentReviewReason = "complete and grounded"
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
	if _, err := store.Get(ctx, "dup-existing-001"); err == nil {
		t.Fatal("existing-content duplicate should not be written to question bank")
	}
	committedItems, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems after commit: %v", err)
	}
	got := committedItems[0]
	if got.Status != ImportItemStatusValid {
		t.Fatalf("duplicate status = %q, want valid", got.Status)
	}
	if got.AgentReviewStatus != ImportAgentReviewRejected || got.AgentReviewReason != qualityFlagDuplicateExistingContent {
		t.Fatalf("duplicate review = %q/%q, want rejected/%s", got.AgentReviewStatus, got.AgentReviewReason, qualityFlagDuplicateExistingContent)
	}
}

func TestImportCommitSkipsFailJobWhenActiveContentReadFails(t *testing.T) {
	ctx := context.Background()
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  failingListWriter{},
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: strings.NewReader(`[{
			"id":"active-read-fail-001",
			"content":"Go channel 关闭后接收行为是什么？",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag"]
		}]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err == nil {
		t.Fatal("Commit should fail when active content key read fails")
	}
	if committed.Status != ImportStatusFailed {
		t.Fatalf("job status = %q, want failed", committed.Status)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("ImportedItems = %d, want 0", committed.ImportedItems)
	}
	summary := requireCommitSummary(t, committed)
	if summary.Matched != 1 || summary.Imported != 0 || summary.FailureReasons["active_content_read_failed"] != 1 {
		t.Fatalf("commit summary = %+v, want active_content_read_failed without imports", summary)
	}
}

func TestImportCommitKeepsJobRecoverableWhenImportItemUpdateFailsAfterPublish(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	baseImports := NewMemoryImportStore()
	imports := &failingUpdateItemsImportStore{MemoryImportStore: baseImports, failUpdateItems: true}
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "questions.json",
		Reader: strings.NewReader(`[{
			"id":"published-before-item-update-fails",
			"content":"Go channel 关闭后接收行为是什么？",
			"skill_category":"go",
			"difficulty":3,
			"tags":["channel"],
			"expected_points":["zero value","ok flag"]
		}]`),
		Size: 256,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err == nil {
		t.Fatal("Commit should report import item update failure")
	}
	if committed.Status != ImportStatusCommitting {
		t.Fatalf("job status = %q, want committing for recoverable post-publish failure", committed.Status)
	}
	if _, getErr := store.Get(ctx, "published-before-item-update-fails"); getErr != nil {
		t.Fatalf("question should remain published after post-publish diagnostic failure: %v", getErr)
	}
	summary := requireCommitSummary(t, committed)
	if summary.Imported != 1 || summary.FailureReasons["import_item_update_failed"] != 1 {
		t.Fatalf("commit summary = %+v, want imported with import_item_update_failed", summary)
	}

	imports.failUpdateItems = false
	recovered, err := service.commitReadyJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("commitReadyJob recovery: %v", err)
	}
	if recovered.Status != ImportStatusCommitted || recovered.ImportedItems != 1 {
		t.Fatalf("recovered job = %+v, want committed with one imported item", recovered)
	}
	if recovered.Error != "" {
		t.Fatalf("recovered job error = %q, want empty", recovered.Error)
	}
	staged, err := baseImports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems after recovery: %v", err)
	}
	if len(staged) != 1 || staged[0].Status != ImportItemStatusImported {
		t.Fatalf("staged items after recovery = %+v, want imported", staged)
	}
	if staged[0].AgentReviewStatus == ImportAgentReviewRejected || staged[0].AgentReviewReason == qualityFlagDuplicateExistingContent {
		t.Fatalf("recovery should not mark self-published item as duplicate: %+v", staged[0])
	}
}

func TestActiveQuestionContentKeysRejectsStuckCursor(t *testing.T) {
	_, err := activeQuestionContentKeys(context.Background(), stuckCursorStore{})
	if err == nil {
		t.Fatal("activeQuestionContentKeys should fail when cursor does not advance")
	}
	if !strings.Contains(err.Error(), "cursor did not advance") {
		t.Fatalf("error = %v, want cursor did not advance", err)
	}
}

func TestReviewAcceptsDocumentGeneratedItemForCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "source.md"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	job, err = service.stageItems(ctx, job, "chunk-1", []Item{completeImportTestItem("doc-accept-001")})
	if err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	staged, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if staged[0].AgentReviewStatus != ImportAgentReviewNeedsHumanReview {
		t.Fatalf("AgentReviewStatus = %q, want needs_human_review", staged[0].AgentReviewStatus)
	}
	questionID := staged[0].Item.ID
	markImportJobReady(t, ctx, imports, job.ID)

	if _, _, err := service.ReviewItems(ctx, job.ID, []string{staged[0].ID}, ImportReviewStatusAccepted); err != nil {
		t.Fatalf("ReviewItems accept: %v", err)
	}
	reviewed, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems reviewed: %v", err)
	}
	if reviewed[0].AgentReviewStatus != ImportAgentReviewAutoApproved {
		t.Fatalf("AgentReviewStatus after accept = %q, want auto_approved", reviewed[0].AgentReviewStatus)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 1 {
		t.Fatalf("ImportedItems = %d, want 1", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, questionID); err != nil {
		t.Fatalf("accepted document item should be written to question bank: %v", err)
	}
}

func TestReviewRejectsDocumentGeneratedItemForCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "source.md"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	job, err = service.stageItems(ctx, job, "chunk-1", []Item{completeImportTestItem("doc-reject-001")})
	if err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	staged, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	questionID := staged[0].Item.ID
	markImportJobReady(t, ctx, imports, job.ID)

	if _, _, err := service.ReviewItems(ctx, job.ID, []string{staged[0].ID}, ImportReviewStatusRejected); err != nil {
		t.Fatalf("ReviewItems reject: %v", err)
	}
	reviewed, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems reviewed: %v", err)
	}
	if reviewed[0].AgentReviewStatus != ImportAgentReviewRejected {
		t.Fatalf("AgentReviewStatus after reject = %q, want rejected", reviewed[0].AgentReviewStatus)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 0 {
		t.Fatalf("ImportedItems = %d, want 0", committed.ImportedItems)
	}
	if _, err := store.Get(ctx, questionID); err == nil {
		t.Fatal("rejected document item should not be written to question bank")
	}
}

func TestDocumentImportStagesUniqueGeneratedIDsAcrossChunks(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})
	job, err := imports.CreateJob(ctx, newImportJob(ImportSourceDocument, "source.md"))
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	job, err = service.stageItems(ctx, job, "chunk-1", []Item{completeImportTestItem("generated-go-001")})
	if err != nil {
		t.Fatalf("stageItems chunk-1: %v", err)
	}
	second := completeImportTestItem("generated-go-001")
	second.Content = "Go 服务如何定位线上内存持续上涨？"
	second.ExpectedPoints = []string{"heap profile", "allocation hot path", "release verification"}
	second.SampleAnswer = "可以先观察 RSS 和 heap 指标，再用 pprof heap 定位分配热点，并验证修复后内存曲线。"
	second.FollowUpHints = []string{"如何区分内存泄漏和缓存增长？"}
	job, err = service.stageItems(ctx, job, "chunk-2", []Item{second})
	if err != nil {
		t.Fatalf("stageItems chunk-2: %v", err)
	}

	staged, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(staged) != 2 {
		t.Fatalf("staged items = %d, want 2", len(staged))
	}
	seenImportIDs := map[string]struct{}{}
	seenQuestionIDs := map[string]struct{}{}
	for _, item := range staged {
		if _, ok := seenImportIDs[item.ID]; ok {
			t.Fatalf("duplicate import item id %q", item.ID)
		}
		if _, ok := seenQuestionIDs[item.Item.ID]; ok {
			t.Fatalf("duplicate generated question id %q", item.Item.ID)
		}
		seenImportIDs[item.ID] = struct{}{}
		seenQuestionIDs[item.Item.ID] = struct{}{}
	}
	markImportJobReady(t, ctx, imports, job.ID)

	if _, _, err := service.ReviewAllValidItems(ctx, job.ID, ImportReviewStatusAccepted, true); err != nil {
		t.Fatalf("ReviewAllValidItems: %v", err)
	}
	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.ImportedItems != 2 {
		t.Fatalf("ImportedItems = %d, want 2", committed.ImportedItems)
	}
	for id := range seenQuestionIDs {
		if _, err := store.Get(ctx, id); err != nil {
			t.Fatalf("committed generated item %q missing from question bank: %v", id, err)
		}
	}
}

func TestDocumentChunkStagingDoesNotMarkJobReadyBeforeProcessCompletes(t *testing.T) {
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

	if _, err := service.stageItems(ctx, job, "chunk-1", []Item{completeImportTestItem("generated-go-001")}); err != nil {
		t.Fatalf("stageItems: %v", err)
	}
	got, err := imports.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status == ImportStatusReady {
		t.Fatal("document chunk staging must not expose job as ready before all chunks complete")
	}
}

func markImportJobReady(t *testing.T, ctx context.Context, imports ImportStore, jobID string) {
	t.Helper()
	job, err := imports.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	job.Status = ImportStatusReady
	if _, err := imports.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob ready: %v", err)
	}
}

func completeImportTestItem(id string) Item {
	return Item{
		ID:             id,
		Content:        "Go 服务如何排查线上 goroutine 泄漏？",
		SkillCategory:  "go",
		Difficulty:     3,
		Tags:           []string{"go", "debugging"},
		ExpectedPoints: []string{"pprof", "context cancellation", "blocked channel"},
		Rubric: map[string]string{
			"pass": "能说出 pprof 和阻塞排查",
		},
		SampleAnswer:  "可以使用 pprof goroutine 查看堆栈，结合 context 取消和 channel 阻塞点定位。",
		FollowUpHints: []string{"如何判断 goroutine 阻塞在 channel send 还是 receive？"},
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

func TestParseQuestionBankItemsToleratesLLMScalarDrift(t *testing.T) {
	raw := []byte(`{"items":[{
		"id":"go-001",
		"content":"请解释 Go slice 的 len 和 cap。",
		"tags":"go,slice",
		"skill_category":"go",
		"difficulty":"4",
		"expected_points":"说明 len；说明 cap",
		"rubric":"能说明底层数组和扩容",
		"sample_answer":"slice 包含指针、长度和容量。",
		"follow_up_hints":"扩容规则是什么？"
	}]}`)
	items, err := parseQuestionBankItems("generated.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems: %v", err)
	}
	if got := items[0].Difficulty; got != 4 {
		t.Fatalf("Difficulty = %d, want 4", got)
	}
	if got := items[0].Rubric["general"]; got != "能说明底层数组和扩容" {
		t.Fatalf("Rubric general = %q", got)
	}
	if got := items[0].ExpectedPoints; len(got) != 2 || got[0] != "说明 len" {
		t.Fatalf("ExpectedPoints = %+v", got)
	}
}

func TestParseQuestionBankItemsAcceptsVersionedImportPackage(t *testing.T) {
	raw, err := os.ReadFile("testdata/question_bank_import_v1.json")
	if err != nil {
		t.Fatalf("ReadFile contract fixture: %v", err)
	}
	items, err := parseQuestionBankItems("contract.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "go-contract-001" {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Difficulty != 4 {
		t.Fatalf("Difficulty = %d, want 4", items[0].Difficulty)
	}
	if got := items[0].Rubric["point_1"]; got != "说明 fatal error" {
		t.Fatalf("rubric point_1 = %q", got)
	}
	if got := items[0].Tags; len(got) != 3 || got[1] != "map" {
		t.Fatalf("Tags = %+v", got)
	}
	if got := items[0].ExpectedPoints; len(got) != 3 || got[2] != "sync.Map 或锁" {
		t.Fatalf("ExpectedPoints = %+v", got)
	}
}

func TestParseQuestionBankItemsRejectsUnsupportedSchemaVersion(t *testing.T) {
	raw := []byte(`{
		"schema_version":"question_bank_import.v9",
		"items":[{"id":"bad-schema","content":"Go map 并发读写为什么不安全？"}]
	}`)
	_, err := parseQuestionBankItems("contract.json", raw)
	if err == nil {
		t.Fatal("parseQuestionBankItems should reject unsupported schema_version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error = %v, want schema_version context", err)
	}
}

func TestParseQuestionBankItemsReportsTopLevelContractFieldPath(t *testing.T) {
	raw := []byte(`{
		"schema_version":{"version":"question_bank_import.v1"},
		"items":[{"id":"bad-schema-type","content":"Go map 并发读写为什么不安全？"}]
	}`)
	_, err := parseQuestionBankItems("contract.json", raw)
	if err == nil {
		t.Fatal("parseQuestionBankItems should reject bad schema_version shape")
	}
	if !strings.Contains(err.Error(), "$.schema_version") {
		t.Fatalf("error = %v, want top-level schema_version path", err)
	}
	if strings.Contains(err.Error(), "$.items[].schema_version") {
		t.Fatalf("error = %v, should not report schema_version as item field", err)
	}
}

func TestParseQuestionBankItemsAcceptsLegacyJSONArray(t *testing.T) {
	raw := []byte(`[{
		"id":"legacy-array-001",
		"content":"Go channel 关闭后读会发生什么？",
		"skill_category":"go",
		"difficulty":3
	}]`)
	items, err := parseQuestionBankItems("legacy.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems: %v", err)
	}
	if len(items) != 1 || items[0].ID != "legacy-array-001" {
		t.Fatalf("items = %+v", items)
	}
}

func TestParseQuestionBankItemsAcceptsLegacyWrappedItemsWithExtraMetadata(t *testing.T) {
	raw := []byte(`{
		"source_ref":{"legacy":"ignored"},
		"review_policy":{"legacy":"ignored"},
		"items":[{
			"id":"legacy-wrapped-001",
			"content":"Redis 缓存穿透怎么处理？",
			"skill_category":"redis",
			"difficulty":3
		}]
	}`)
	items, err := parseQuestionBankItems("legacy.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems should keep legacy wrapper lenient: %v", err)
	}
	if len(items) != 1 || items[0].ID != "legacy-wrapped-001" {
		t.Fatalf("items = %+v", items)
	}
}

func TestParseQuestionBankItemsAcceptsLegacyCSVAndMarkdown(t *testing.T) {
	csvItems, err := parseQuestionBankItems("legacy.csv", []byte("id,content,tags,skill_category,difficulty,expected_points\ncsv-001,Go defer 顺序是什么,go；defer,go,3,LIFO；参数立即求值\n"))
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(csvItems) != 1 || csvItems[0].ID != "csv-001" || len(csvItems[0].ExpectedPoints) != 2 {
		t.Fatalf("csv items = %+v", csvItems)
	}

	mdItems, err := parseQuestionBankItems("legacy.md", []byte("## Go map 并发读写为什么不安全？\n需要说明 runtime fatal error 和锁保护。\n"))
	if err != nil {
		t.Fatalf("parse markdown: %v", err)
	}
	if len(mdItems) != 1 || !strings.Contains(mdItems[0].Content, "runtime fatal error") {
		t.Fatalf("markdown items = %+v", mdItems)
	}
}

func TestParseQuestionBankItemsToleratesRubricArray(t *testing.T) {
	raw := []byte(`{"items":[{
		"id":"go-rubric-array-001",
		"content":"Go defer 的执行顺序是什么？",
		"skill_category":"go",
		"difficulty":3,
		"rubric":["说明 LIFO","说明参数立即求值"]
	}]}`)
	items, err := parseQuestionBankItems("generated.json", raw)
	if err != nil {
		t.Fatalf("parseQuestionBankItems: %v", err)
	}
	if got := items[0].Rubric["point_1"]; got != "说明 LIFO" {
		t.Fatalf("Rubric point_1 = %q", got)
	}
	if got := items[0].Rubric["point_2"]; got != "说明参数立即求值" {
		t.Fatalf("Rubric point_2 = %q", got)
	}
}

func TestParseQuestionBankItemsRejectsUnsupportedFlexibleStringList(t *testing.T) {
	raw := []byte(`{"items":[{
		"id":"go-bad-tags-001",
		"content":"Go map 的并发安全问题是什么？",
		"skill_category":"go",
		"difficulty":3,
		"tags":{"primary":"go"}
	}]}`)
	_, err := parseQuestionBankItems("generated.json", raw)
	if err == nil {
		t.Fatal("parseQuestionBankItems should reject unsupported tags shape")
	}
	if !strings.Contains(err.Error(), "tags") {
		t.Fatalf("error = %v, want tags context", err)
	}
}

func TestParseQuestionBankItemsReportsFlexibleFieldNameOnBadType(t *testing.T) {
	raw := []byte(`{"items":[{
		"id":"bad-expected-points",
		"content":"Redis 热 key 如何治理？",
		"skill_category":"redis",
		"difficulty":3,
		"expected_points":{"primary":"发现热 key"}
	}]}`)
	_, err := parseQuestionBankItems("generated.json", raw)
	if err == nil {
		t.Fatal("parseQuestionBankItems should reject bad expected_points shape")
	}
	if !strings.Contains(err.Error(), "expected_points") {
		t.Fatalf("error = %v, want expected_points context", err)
	}
	if !strings.Contains(err.Error(), "$.items[].expected_points") {
		t.Fatalf("error = %v, want field path context", err)
	}
	if !strings.Contains(err.Error(), `raw={"primary":"发现热 key"}`) {
		t.Fatalf("error = %v, want raw value summary", err)
	}
}

func TestImportService_ImportLocalQuestionBankPreservesVersionedPackageMetadata(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports: imports,
		Writer:  store,
	})
	raw, err := os.ReadFile("testdata/question_bank_import_v1.json")
	if err != nil {
		t.Fatalf("ReadFile contract fixture: %v", err)
	}

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename:    "contract.json",
		ContentType: "application/json",
		Reader:      bytes.NewReader(raw),
		Size:        int64(len(raw)),
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if job.Metadata["schema_version"] != questionBankImportSchemaV1 {
		t.Fatalf("job metadata = %+v, want schema_version", job.Metadata)
	}
	if job.Metadata["source_ref"] != "obsidian/go.md" {
		t.Fatalf("job metadata = %+v, want source_ref", job.Metadata)
	}
	if !strings.Contains(job.Metadata["review_policy"], "needs_human_review") {
		t.Fatalf("job metadata = %+v, want compact review_policy", job.Metadata)
	}
	items, err := imports.ListItems(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one staged item", items)
	}
	provenance := items[0].SourceProvenance
	if provenance["source_type"] != ImportSourceQuestionBank || provenance["schema_version"] != questionBankImportSchemaV1 {
		t.Fatalf("source provenance = %+v, want question bank schema facts", provenance)
	}
	if provenance["source_ref"] != "obsidian/go.md" || provenance["source_hash"] == "" {
		t.Fatalf("source provenance = %+v, want source_ref and source_hash", provenance)
	}
	if _, ok := items[0].FieldProvenance["schema_version"]; ok {
		t.Fatalf("field provenance should not contain package metadata: %+v", items[0].FieldProvenance)
	}
	if _, ok := items[0].FieldProvenance["source_hash"]; ok {
		t.Fatalf("field provenance should not contain source metadata: %+v", items[0].FieldProvenance)
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
		FieldProvenance:   map[string]string{"difficulty": "generated"},
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

func TestPGImportItemErrorsForDBUsesEmptyArray(t *testing.T) {
	got := pgImportItemErrorsForDB(nil)
	if got == nil {
		t.Fatal("nil import errors should be stored as empty array, not SQL NULL")
	}
	if len(got) != 0 {
		t.Fatalf("errors = %+v, want empty", got)
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
	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
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
	summary := requireCommitSummary(t, committed)
	if summary.EmbeddingSynced != 1 || summary.EmbeddingFailed != 0 {
		t.Fatalf("embedding summary = %+v, want synced=1 failed=0", summary)
	}
}

func TestImportService_CommitRecordsPublishSummary(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(nil)
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{Imports: imports, Writer: store})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "summary.json",
		Reader: bytes.NewBufferString(`[
			{"id":"summary-import-001","content":"Go channel 泄漏如何排查？","skill_category":"go","difficulty":3},
			{"id":"summary-reject-001","content":"Go mutex 饥饿模式是什么？","skill_category":"go","difficulty":4},
			{"id":"summary-invalid-001","content":"","skill_category":"go","difficulty":2},
			{"id":"summary-dup-a","content":"Redis 热 key 如何发现和治理？","skill_category":"redis","difficulty":4},
			{"id":"summary-dup-b","content":"Redis 热 key 如何发现和治理？","skill_category":"redis","difficulty":4},
			{"id":"summary-dirty-001","content":"有使用过吗你这个agent项目就是四个智能体 用langchain也可以实现啊 你用了langGraph 有哪些是用langchain不能实现的吗--（无法反驳..","skill_category":"agent","difficulty":3}
		]`),
		Size: 512,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	if _, _, err := service.ReviewItems(ctx, job.ID, []string{job.ID + ":summary-reject-001"}, ImportReviewStatusRejected); err != nil {
		t.Fatalf("ReviewItems reject: %v", err)
	}

	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	summary := requireCommitSummary(t, committed)
	if summary.Matched != 6 || summary.Imported != 2 || summary.Skipped != 4 {
		t.Fatalf("summary counts = %+v, want matched=6 imported=2 skipped=4", summary)
	}
	if summary.EmbeddingSynced != 0 || summary.EmbeddingFailed != 0 {
		t.Fatalf("embedding counts = %+v, want no embedding work", summary)
	}
	for _, reason := range []string{
		"invalid",
		"review_rejected",
		qualityFlagDuplicateContent,
		QualityFlagDirtyNoteMarker,
	} {
		if summary.FailureReasons[reason] == 0 {
			t.Fatalf("failure reasons = %+v, want %q", summary.FailureReasons, reason)
		}
	}
}

func TestImportService_CommitMarksEmbeddingFailureDiagnostics(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore([]Item{{
		ID:            "embedding-fail-001",
		Content:       "PostgreSQL 旧题内容",
		SkillCategory: "postgresql",
		Difficulty:    3,
	}})
	if err := store.UpsertEmbeddings(ctx, []ItemEmbedding{{
		ID:     "embedding-fail-001",
		Vector: []float32{1, 2, 3, 4, 5, 6, 7, 8},
		Model:  "old-model",
	}}); err != nil {
		t.Fatalf("seed old embedding: %v", err)
	}
	imports := NewMemoryImportStore()
	service := NewImportService(ImportServiceDeps{
		Imports:  imports,
		Writer:   store,
		Embedder: failingEmbedder{err: errors.New("embedding backend unavailable")},
	})

	job, err := service.ImportLocalQuestionBank(ctx, ImportFile{
		Filename: "embedding-fail.json",
		Reader: bytes.NewBufferString(`[
			{"id":"embedding-fail-001","content":"PostgreSQL 慢查询应该如何定位？","skill_category":"postgresql","difficulty":4}
		]`),
		Size: 128,
	})
	if err != nil {
		t.Fatalf("ImportLocalQuestionBank: %v", err)
	}
	committed, err := service.Commit(ctx, job.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if committed.Status != ImportStatusCommitted || committed.ImportedItems != 1 {
		t.Fatalf("committed job = %+v, want committed with one imported item", committed)
	}
	item, err := store.Get(ctx, "embedding-fail-001")
	if err != nil {
		t.Fatalf("committed item should stay in question bank: %v", err)
	}
	if item.EmbeddingStatus != "failed" {
		t.Fatalf("embedding_status = %q, want failed", item.EmbeddingStatus)
	}
	if !strings.Contains(item.EmbeddingError, "embedding backend unavailable") {
		t.Fatalf("embedding_error = %q, want backend error", item.EmbeddingError)
	}
	if _, _, ok := store.Embedding("embedding-fail-001"); ok {
		t.Fatal("failed embedding should clear stale in-memory vector")
	}
	summary := requireCommitSummary(t, committed)
	if summary.EmbeddingSynced != 0 || summary.EmbeddingFailed != 1 {
		t.Fatalf("embedding summary = %+v, want synced=0 failed=1", summary)
	}
	if summary.FailureReasons["embedding_failed"] != 1 {
		t.Fatalf("failure reasons = %+v, want embedding_failed", summary.FailureReasons)
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

type failingListWriter struct{}

func (failingListWriter) List(context.Context, Filter) (ListResult, error) {
	return ListResult{}, errors.New("list active questions failed")
}

func (failingListWriter) Get(context.Context, string) (Item, error) {
	return Item{}, ErrNotFound
}

func (failingListWriter) Facets(context.Context) (Facets, error) {
	return Facets{}, nil
}

func (failingListWriter) Upsert(context.Context, []Item) error {
	return errors.New("upsert should not be called")
}

type stuckCursorStore struct{}

func (stuckCursorStore) List(_ context.Context, filter Filter) (ListResult, error) {
	if filter.Cursor == "" {
		return ListResult{Items: []Item{{ID: "one", Content: "one", Status: "active"}}, NextCursor: "same"}, nil
	}
	return ListResult{Items: []Item{{ID: "two", Content: "two", Status: "active"}}, NextCursor: filter.Cursor}, nil
}

func (stuckCursorStore) Get(context.Context, string) (Item, error) {
	return Item{}, ErrNotFound
}

func (stuckCursorStore) Facets(context.Context) (Facets, error) {
	return Facets{}, nil
}

type failingEmbedder struct {
	err error
}

func (f failingEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, f.err
}

func (f failingEmbedder) Dimension() int { return 8 }
func (f failingEmbedder) Name() string   { return "failing" }

type failingUpdateItemsImportStore struct {
	*MemoryImportStore
	failUpdateItems bool
}

func (s *failingUpdateItemsImportStore) UpdateItems(ctx context.Context, items []ImportItem) error {
	if s.failUpdateItems {
		return errors.New("update import items failed")
	}
	return s.MemoryImportStore.UpdateItems(ctx, items)
}

type commitSummaryForTest struct {
	Matched         int            `json:"matched"`
	Imported        int            `json:"imported"`
	Skipped         int            `json:"skipped"`
	EmbeddingSynced int            `json:"embedding_synced"`
	EmbeddingFailed int            `json:"embedding_failed"`
	FailureReasons  map[string]int `json:"failure_reasons"`
	EmbeddingErrors []string       `json:"embedding_errors"`
}

func requireCommitSummary(t *testing.T, job ImportJob) commitSummaryForTest {
	t.Helper()
	raw := job.Metadata["commit_summary"]
	if raw == "" {
		t.Fatalf("job metadata = %+v, want commit_summary", job.Metadata)
	}
	var summary commitSummaryForTest
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		t.Fatalf("unmarshal commit_summary %q: %v", raw, err)
	}
	if summary.FailureReasons == nil {
		t.Fatal("commit_summary failure_reasons is nil")
	}
	return summary
}
