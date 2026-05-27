package parser

import (
	"context"
	"fmt"
	"strings"
)

// Dispatcher 根据 Hint 选择 PDF 或 DOCX parser。
//
// 为什么不让 handler 直接调具体 parser：
//   - HTTP 入口拿到的就是 file + ContentType，不应该关心实现
//   - 未来加 TXT / MD / RTF 时只改 Dispatcher，不动 handler
//   - 单测可以注入 fake parser 验证路由逻辑
type Dispatcher struct {
	pdf  DocumentParser
	docx DocumentParser
	text DocumentParser
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		pdf:  NewPDFParser(),
		docx: NewDOCXParser(),
		text: NewPlainTextParser(),
	}
}

// NewDispatcherWith 允许测试注入 mock，或运维替换实现。
func NewDispatcherWith(pdf, docx DocumentParser) *Dispatcher {
	return &Dispatcher{pdf: pdf, docx: docx, text: NewPlainTextParser()}
}

func (d *Dispatcher) Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error) {
	p, err := d.pick(hint)
	if err != nil {
		return nil, err
	}
	return p.Parse(ctx, src, hint, limit)
}

// pick 优先信任 ContentType（HTTP header 更权威），
// 兜底用 Filename 扩展名。两者都对不上则 ErrUnsupportedType。
func (d *Dispatcher) pick(hint Hint) (DocumentParser, error) {
	ct := strings.ToLower(hint.ContentType)
	switch {
	case strings.Contains(ct, "pdf"):
		return d.pdf, nil
	case strings.Contains(ct, "wordprocessingml") || strings.Contains(ct, "msword"):
		return d.docx, nil
	case strings.HasPrefix(ct, "text/") || strings.Contains(ct, "markdown"):
		return d.text, nil
	}

	name := strings.ToLower(hint.Filename)
	switch {
	case strings.HasSuffix(name, ".pdf"):
		return d.pdf, nil
	case strings.HasSuffix(name, ".docx"):
		return d.docx, nil
	case strings.HasSuffix(name, ".txt"), strings.HasSuffix(name, ".md"), strings.HasSuffix(name, ".markdown"):
		return d.text, nil
	}

	return nil, fmt.Errorf("%w: content-type=%q filename=%q",
		ErrUnsupportedType, hint.ContentType, hint.Filename)
}
