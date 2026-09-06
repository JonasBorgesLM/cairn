//go:build integration

package redisstore_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/redisstore"
)

// benchRedis starts one Redis container for the calling benchmark, mirroring
// testRedis in integration_test.go -- duplicated rather than shared because
// that helper is typed to *testing.T, and *testing.B needs its own.
func benchRedis(b *testing.B) string {
	b.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		b.Fatalf("starting redis container: %v", err)
	}
	b.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			b.Logf("terminating redis container: %v", err)
		}
	})

	addr, err := container.Endpoint(ctx, "")
	if err != nil {
		b.Fatalf("getting redis endpoint: %v", err)
	}
	return addr
}

// BenchmarkShortener_Resolve_Redis measures the full Resolve path against a
// real, local Redis (NFR-10) -- the realistic denominator issue #50 asks for,
// as distinct from BenchmarkShortener_Resolve_Memstore's in-process floor.
// docs/benchmarks.md computes ADR-0007's unmask-as-a-percentage-of-redirect-
// latency figure against this number, not the memstore one: a real store
// round trip is what "redirect-path latency" means in production, and
// memstore's near-zero cost would make the unmask look disproportionately
// large by comparison.
func BenchmarkShortener_Resolve_Redis(b *testing.B) {
	addr := benchRedis(b)
	store, err := redisstore.New(addr)
	if err != nil {
		b.Fatalf("redisstore.New error = %v", err)
	}
	b.Cleanup(func() { _ = store.Close() })

	s, err := cairn.New(store, cairn.WithPolicy(benchAllowPolicy{}))
	if err != nil {
		b.Fatalf("cairn.New error = %v", err)
	}

	for _, n := range []int{32, 128, 512, 2000} {
		b.Run(fmt.Sprintf("%dbytes", n), func(b *testing.B) {
			path := strings.Repeat("a", max(n-len("https://example.com/"), 1))
			link, err := s.Create(context.Background(), "https://example.com/"+path)
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
		})
	}
}

type benchAllowPolicy struct{}

func (benchAllowPolicy) Evaluate(context.Context, cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	return cairn.Allow, "", nil
}
