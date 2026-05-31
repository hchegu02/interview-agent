package questionbank

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
