package cairn_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// BenchmarkDestination_URL measures the unmask ADR-0007 objection 3 names:
// Destination.URL() runs an HMAC-SHA-256 keystream derivation over the raw
// URL, and every redirect calls it to build the Location header. Lengths
// span realistic short-to-long destinations up to SR-08's 2000-byte ceiling,
// since ADR-0007's reopening criterion is stated at "realistic URL lengths",
// not one point.
func BenchmarkDestination_URL(b *testing.B) {
	for _, n := range []int{32, 128, 512, 2000} {
		b.Run(fmt.Sprintf("%dbytes", n), func(b *testing.B) {
			path := strings.Repeat("a", n-len("https://example.com/"))
			dest, err := cairn.ParseDestination("https://example.com/" + path)
			if err != nil {
				b.Fatalf("ParseDestination error = %v", err)
			}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := dest.URL(); err != nil {
					b.Fatalf("URL error = %v", err)
				}
			}
		})
	}
}
