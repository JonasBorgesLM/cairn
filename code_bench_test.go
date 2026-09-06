package cairn_test

import (
	"context"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// BenchmarkNewRandomGenerator_Generate measures code generation: one
// crypto/rand draw plus rejection sampling (SR-01, ADR-0002) per rune.
func BenchmarkNewRandomGenerator_Generate(b *testing.B) {
	g, err := cairn.NewRandomGenerator(cairn.AlphabetBase62, 10)
	if err != nil {
		b.Fatalf("NewRandomGenerator error = %v", err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := g.Generate(ctx); err != nil {
			b.Fatalf("Generate error = %v", err)
		}
	}
}
