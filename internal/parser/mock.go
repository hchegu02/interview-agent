package parser

import (
	"context"
	"errors"
)

// MockParser 让 Graph 节点单测无需准备真实 PDF/DOCX 文件。
// 注入到 Dispatcher 时通过 NewDispatcherWith。
type MockParser struct {
	Text       string
	PageCount  int
	ReturnErr  error
	CallCount  int
	LastSource Source
	LastHint   Hint
	LastLimit  ParseLimit
}

func (m *MockParser) Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error) {
	m.CallCount++
	m.LastSource = src
	m.LastHint = hint
	m.LastLimit = limit
	if m.ReturnErr != nil {
		return nil, m.ReturnErr
	}
	if m.Text == "" {
		return nil, errors.New("mock parser: empty text configured")
	}
	return &Document{
		Text:      m.Text,
		PageCount: m.PageCount,
		Metadata:  map[string]string{"format": "mock"},
	}, nil
}
