// Package parser 解析候选人简历 / 岗位 JD / 题库 PDF/DOCX。
//
// 核心设计决策：
//
//  1. 接口接 io.ReaderAt + Size，不接 io.Reader。
//     PDF 格式 xref 表在文件末尾，必须随机访问；DOCX 是 zip
//     同样要求 ReaderAt。强行流式只能 io.ReadAll 进内存，不如
//     让调用方显式传 ReaderAt + 大小，便于前置大小检查。
//
//  2. ParseLimit 作为入参而非 parser 字段。
//     同一个 parser 实例服务多个入口（简历 10MB / JD 5MB /
//     题库导入 100MB），policy 在 handler 层选定后注入。
//
//  3. Document 返回纯文本 + 元数据，不返回原始结构。
//     LLM 抽取画像只需要文本；保留 PageCount / Metadata 留扩展。
package parser

import (
	"context"
	"errors"
	"io"
	"time"
)

// Source 是可随机访问的字节流，对应 multipart.File / *os.File / *bytes.Reader。
type Source struct {
	Data io.ReaderAt
	Size int64
}

// Hint 给 Dispatcher 用来选择具体实现。
type Hint struct {
	Filename    string // "resume.pdf"，主要用来兜底
	ContentType string // "application/pdf"，从 HTTP header 拿
}

// ParseLimit 控制单次解析的资源边界。
//
// 为什么用结构体而不是单参数：未来加 MaxImages / MaxTables
// 等约束时不破坏接口签名。
type ParseLimit struct {
	MaxBytes int64         // 超过直接拒绝，不进 parser
	MaxPages int           // 解析到此页停止
	Timeout  time.Duration // ctx 超时，防止恶意 PDF 卡死
}

// Document 是解析结果。Text 已做空白归一化。
type Document struct {
	Text      string
	PageCount int
	Metadata  map[string]string
}

// DocumentParser 是 PDF / DOCX 实现的统一抽象。
type DocumentParser interface {
	Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error)
}

// 预设三档 policy，供 handler 层引用。
// 数值留有富裕：简历正常 < 2MB，10MB 已经能容纳带嵌入字体的扫描件首页。
var (
	LimitResume = ParseLimit{
		MaxBytes: 10 << 20, // 10 MB
		MaxPages: 20,
		Timeout:  5 * time.Second,
	}
	LimitJD = ParseLimit{
		MaxBytes: 5 << 20, // 5 MB
		MaxPages: 10,
		Timeout:  3 * time.Second,
	}
	LimitQuestionBankImport = ParseLimit{
		MaxBytes: 100 << 20, // 100 MB，仅限 admin 离线导入
		MaxPages: 500,
		Timeout:  60 * time.Second,
	}
)

// 错误集中定义，便于 handler 层 errors.Is 判断后映射 HTTP code。
var (
	ErrTooLarge        = errors.New("parser: file exceeds size limit")
	ErrTooManyPages    = errors.New("parser: page count exceeds limit")
	ErrUnsupportedType = errors.New("parser: unsupported content type")
	ErrEmptyDocument   = errors.New("parser: document is empty")
	ErrInvalidFormat   = errors.New("parser: invalid file format")
)
