package embedding

import (
	"context"
	"math"
	"testing"
)

func TestMockEmbedder_Deterministic(t *testing.T) {
	m := NewMockEmbedder(768)
	a, _ := m.Embed(context.Background(), []string{"hello"})
	b, _ := m.Embed(context.Background(), []string{"hello"})
	if len(a) != 1 || len(b) != 1 {
		t.Fatal("expected 1 vector")
	}
	for i := range a[0] {
		if a[0][i] != b[0][i] {
			t.Fatalf("non-deterministic at idx %d", i)
		}
	}
}

func TestMockEmbedder_Dimension(t *testing.T) {
	m := NewMockEmbedder(0) // default to question_bank.embedding vector(1024)
	if m.Dimension() != 1024 {
		t.Errorf("expected 1024, got %d", m.Dimension())
	}
}

func TestMockEmbedder_Normalized(t *testing.T) {
	m := NewMockEmbedder(768)
	vs, _ := m.Embed(context.Background(), []string{"normalize me"})
	var sum float64
	for _, x := range vs[0] {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if math.Abs(norm-1.0) > 1e-3 {
		t.Errorf("expected unit norm, got %f", norm)
	}
}
