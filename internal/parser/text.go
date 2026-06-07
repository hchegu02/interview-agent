package parser

import (
	"context"
	"fmt"
	"io"
)

// PlainTextParser handles already-textual resume files such as .txt and .md.
type PlainTextParser struct{}

func NewPlainTextParser() *PlainTextParser { return &PlainTextParser{} }

func (p *PlainTextParser) Parse(ctx context.Context, src Source, hint Hint, limit ParseLimit) (*Document, error) {
	if src.Size > limit.MaxBytes {
		return nil, fmt.Errorf("%w: got %d bytes, max %d", ErrTooLarge, src.Size, limit.MaxBytes)
	}
	ctx, cancel := context.WithTimeout(ctx, limit.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data := make([]byte, src.Size)
	reader := io.NewSectionReader(src.Data, 0, src.Size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, fmt.Errorf("%w: read text: %v", ErrInvalidFormat, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	body := PostProcessText(string(data), PostProcessOptions{Kind: "resume"}).Text
	if body == "" {
		return nil, ErrEmptyDocument
	}
	return &Document{
		Text:      body,
		PageCount: 1,
		Metadata: map[string]string{
			"format": "text",
		},
	}, nil
}
