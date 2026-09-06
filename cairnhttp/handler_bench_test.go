package cairnhttp_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/memstore"
)

// BenchmarkHandler_ServeHTTP measures the full redirect path -- resolve,
// header writes, the Destination.URL() unmask, WriteHeader -- at the same
// destination lengths as BenchmarkDestination_URL, so the unmask's cost can
// be expressed as a percentage of this: the reopening criterion ADR-0007
// names (docs/benchmarks.md computes it from these two benchmarks' output).
func BenchmarkHandler_ServeHTTP(b *testing.B) {
	for _, n := range []int{32, 128, 512, 2000} {
		b.Run(fmt.Sprintf("%dbytes", n), func(b *testing.B) {
			s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}))
			if err != nil {
				b.Fatalf("cairn.New error = %v", err)
			}
			path := strings.Repeat("a", max(n-len("https://example.com/"), 1))
			link, err := s.Create(context.Background(), "https://example.com/"+path)
			if err != nil {
				b.Fatalf("Create error = %v", err)
			}
			h := cairnhttp.NewHandler(s)
			b.Cleanup(func() { _ = h.Close() })
			target := "/" + string(link.Code)

			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
			}
		})
	}
}
