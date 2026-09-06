package cairn_test

import (
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// BenchmarkAlphabetBase62_Validate measures SR-21's gate: validation happens
// before a code is ever concatenated into a store's keyspace, so its cost is
// paid on every resolve, including a scanner's malformed probes.
func BenchmarkAlphabetBase62_Validate(b *testing.B) {
	code := cairn.Code("abc1234567")

	b.ReportAllocs()
	for b.Loop() {
		if err := cairn.AlphabetBase62.Validate(code); err != nil {
			b.Fatalf("Validate error = %v", err)
		}
	}
}
