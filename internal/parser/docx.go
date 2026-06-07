package parser

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// DOCXParser 直接用 archive/zip + encoding/xml 解析 .docx。
//
// 为什么不引第三方 DOCX 库：
//   - nguyenthenguyen/docx 主打"模板填充"，文本提取是副业
//   - unioffice 是商业 license
//   - fumiama/go-docx 较新但维护人少
//
// .docx 本质就是 zip，里面 word/document.xml 用 <w:t> 标签包文字。
// 我们只需要：解压 -> 找 document.xml -> 流式扫描 <w:t> 文本节点。
// 整个实现 < 80 行，没有任何外部依赖，故障域完全自有。
//
// 这是个面试加分点：能用标准库解决就不引依赖，依赖即负债。
type DOCXParser struct{}

func NewDOCXParser() *DOCXParser { return &DOCXParser{} }

func (p *DOCXParser) Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error) {
	if src.Size > limit.MaxBytes {
		return nil, fmt.Errorf("%w: got %d bytes, max %d", ErrTooLarge, src.Size, limit.MaxBytes)
	}

	ctx, cancel := context.WithTimeout(ctx, limit.Timeout)
	defer cancel()

	zr, err := zip.NewReader(src.Data, src.Size)
	if err != nil {
		return nil, fmt.Errorf("%w: not a valid zip: %v", ErrInvalidFormat, err)
	}

	var docXML *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docXML = f
			break
		}
	}
	if docXML == nil {
		return nil, fmt.Errorf("%w: missing word/document.xml", ErrInvalidFormat)
	}

	rc, err := docXML.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open document.xml: %v", ErrInvalidFormat, err)
	}
	defer rc.Close()

	text, paragraphs, err := extractDOCXText(ctx, rc)
	if err != nil {
		return nil, err
	}

	// DOCX 没有"页"的硬概念（页是渲染时才确定的），
	// 我们用段落数 / 30 估算页数，30 是 A4 默认排版的经验值。
	estimatedPages := paragraphs/30 + 1
	if estimatedPages > limit.MaxPages {
		return nil, fmt.Errorf("%w: estimated %d pages, max %d", ErrTooManyPages, estimatedPages, limit.MaxPages)
	}

	body := PostProcessText(text, PostProcessOptions{Kind: "resume"}).Text
	if body == "" {
		return nil, ErrEmptyDocument
	}

	return &Document{
		Text:      body,
		PageCount: estimatedPages,
		Metadata: map[string]string{
			"format":     "docx",
			"paragraphs": fmt.Sprintf("%d", paragraphs),
		},
	}, nil
}

// extractDOCXText 流式扫描 XML，提取 <w:t> 文本和 </w:p> 段落分隔。
// 用 xml.Decoder.Token() 而非 Unmarshal 进结构体：document.xml 嵌套
// 很深、schema 复杂，流式扫描比定义匹配的 Go 类型省一千行代码。
func extractDOCXText(ctx context.Context, r io.Reader) (string, int, error) {
	dec := xml.NewDecoder(r)
	var sb strings.Builder
	var inText bool
	paragraphs := 0
	tokenCount := 0

	for {
		// 每 1000 个 token 检查一次 ctx，平衡及时性和开销
		tokenCount++
		if tokenCount%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return "", 0, err
			}
		}

		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, fmt.Errorf("%w: xml parse: %v", ErrInvalidFormat, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				// 段落结束补一个换行，保留段落语义
				sb.WriteByte('\n')
				paragraphs++
			}
		}
	}

	return sb.String(), paragraphs, nil
}
