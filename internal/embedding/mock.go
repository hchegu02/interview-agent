package embedding

import (
	"context"
	"hash/fnv"
	"math"
)

// MockEmbedder 用 FNV hash + 正弦扰动生成确定性伪向量。
//
// 关键点：
//   - 同样的文本始终产出同样的向量（可复现的单测）
//   - 维度可配置，默认 1024 与 question_bank.embedding vector(1024) 保持一致
//   - 向量做了 L2 归一化，使得 cosine 距离行为更符合直觉
type MockEmbedder struct {
	Dim int
}

func NewMockEmbedder(dim int) *MockEmbedder {
	if dim <= 0 {
		dim = 1024
	}
	return &MockEmbedder{Dim: dim}
}

func (m *MockEmbedder) Dimension() int { return m.Dim }
func (m *MockEmbedder) Name() string   { return "mock" }

func (m *MockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		out[i] = m.vector(t)
	}
	return out, nil
}

func (m *MockEmbedder) vector(text string) []float32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()
	v := make([]float32, m.Dim)
	var norm float64
	for i := 0; i < m.Dim; i++ {
		// 基于 seed + index 的伪随机
		x := float64((seed>>(uint(i)%64))&0xff) - 128.0
		x += math.Sin(float64(i) + float64(seed%97))
		v[i] = float32(x / 128.0)
		norm += float64(v[i]) * float64(v[i])
	}
	// L2 归一化
	if norm > 0 {
		n := float32(math.Sqrt(norm))
		for i := range v {
			v[i] /= n
		}
	}
	return v
}
