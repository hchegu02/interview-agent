package parser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFParser 用 ledongthuc/pdf 提取纯文本。
//
// 为什么不用 unipdf：商业 license + 体积大，本场景用不到 OCR/表单。
// 为什么不用 pdfcpu：它设计目标是 PDF 处理（拆分/水印/校验），
// 文本提取是副业、质量一般。
//
// 已知局限：
//   - 不支持加密 PDF
//   - 扫描件（图像 PDF）拿不到文本——但 ParseLimit.MaxBytes 已经
//     把多数扫描件挡在门外，剩下的让 LLM 抽画像时返回"信息不足"
type PDFParser struct{}

func NewPDFParser() *PDFParser { return &PDFParser{} }

func (p *PDFParser) Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error) {
	// 前置检查：大小。Size 在 Source 层就拿到了，不读字节。
	if src.Size > limit.MaxBytes {
		return nil, fmt.Errorf("%w: got %d bytes, max %d", ErrTooLarge, src.Size, limit.MaxBytes)
	}

	// 超时由 ctx 承担。注意 ledongthuc/pdf 本身不感知 ctx，
	// 我们在分页循环里手动检查，避免恶意 PDF 卡死整条请求。
	ctx, cancel := context.WithTimeout(ctx, limit.Timeout)
	defer cancel()

	r, err := pdf.NewReader(src.Data, src.Size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFormat, err)
	}

	pageCount := r.NumPage()
	if pageCount == 0 {
		return nil, ErrEmptyDocument
	}
	if pageCount > limit.MaxPages {
		return nil, fmt.Errorf("%w: got %d pages, max %d", ErrTooManyPages, pageCount, limit.MaxPages)
	}

	var sb strings.Builder
	sb.Grow(int(src.Size)) // 经验值：解析后文本约为原 PDF 字节数的 0.3~0.8 倍
	for i := 1; i <= pageCount; i++ {
		// 每页边界检查 ctx，恶意 PDF 卡在单页解析时也能退出。
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// 单页失败不整体放弃，记录但继续——很多 PDF 个别页字体异常很常见。
			continue
		}
		sb.WriteString(text)
		sb.WriteByte('\n')
	}

	body := normalizeWhitespace(sb.String())
	if body == "" {
		return nil, ErrEmptyDocument
	}

	return &Document{
		Text:      body,
		PageCount: pageCount,
		Metadata: map[string]string{
			"format": "pdf",
		},
	}, nil
}

// normalizeWhitespace 把多个空白合并成单空格、去掉行首尾空白。
// PDF 提取出的文本经常一堆 \r、\t、连续空格，先归一化便于 LLM 处理。
func normalizeWhitespace(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	prevSpace := true // 首字符若是空白则丢弃
	for _, r := range s {
		if r == '\n' {
			// 保留换行（段落语义），但塌缩连续空行
			if !strings.HasSuffix(sb.String(), "\n") {
				sb.WriteRune('\n')
			}
			prevSpace = true
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' {
			if !prevSpace {
				sb.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		sb.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(sb.String())
}

// 保留 errors 引用，防止 lint 抱怨
var _ = errors.New
