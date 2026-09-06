package cairn_test

import (
	"context"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/memstore"
)

// BenchmarkShortener_Resolve_Memstore measures the full Resolve path --
// alphabet validation, store load, revoked/expired checks -- against
// memstore, giving the in-process floor a Redis-backed Resolve is measured
// against.
func BenchmarkShortener_Resolve_Memstore(b *testing.B) {
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}))
	if err != nil {
		b.Fatalf("cairn.New error = %v", err)
	}
	link, err := s.Create(context.Background(), "https://example.com/docs")
	if err != nil {
		b.Fatalf("Create error = %v", err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Resolve(ctx, link.Code); err != nil {
			b.Fatalf("Resolve error = %v", err)
		}
	}
}
