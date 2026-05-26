// Package embedding 把文本转成向量。
//
// 设计：和 llm 包同样用 Mock/Real 双实现。
// Mock 用确定性 hash 生成固定维度向量，保证测试可复现；
// 维度必须和数据库 question_bank.embedding vector(N) 一致，否则会写入失败。
package embedding

import (
	"context"
	"errors"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
	Name() string
}

var ErrDimensionMismatch = errors.New("embedding dimension mismatch")
