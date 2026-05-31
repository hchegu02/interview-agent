package questionbank

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
)

var ErrImportServiceShutdown = errors.New("question bank import service is shut down")

func (s *ImportService) EnqueueImport(ctx context.Context, sourceType string, file ImportFile) (ImportJob, error) {
	if s == nil || s.imports == nil {
		return ImportJob{}, errors.New("question bank import store not configured")
	}
	if s.isShutdown() {
		return ImportJob{}, ErrImportServiceShutdown
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
	if s == nil || s.imports == nil {
		return ImportJob{}, errors.New("question bank import store not configured")
	}
	if s.isShutdown() {
		return ImportJob{}, ErrImportServiceShutdown
	}
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

func (s *ImportService) runAsync(fn func()) bool {
	if s == nil {
		return false
	}
	s.asyncMu.Lock()
	if s.asyncClosed {
		s.asyncMu.Unlock()
		return false
	}
	s.asyncWG.Add(1)
	s.asyncMu.Unlock()

	go func() {
		defer s.asyncWG.Done()
		s.workers <- struct{}{}
		defer func() { <-s.workers }()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("question bank import background task panicked: %v", recovered)
			}
		}()
		fn()
	}()
	return true
}

func (s *ImportService) isShutdown() bool {
	if s == nil {
		return true
	}
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()
	return s.asyncClosed
}

func (s *ImportService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.asyncMu.Lock()
	s.asyncClosed = true
	s.asyncMu.Unlock()

	done := make(chan struct{})
	go func() {
		s.asyncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
