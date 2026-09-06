package cairn

import (
	"context"
	"errors"
	"testing"
)

type failingReader struct{ err error }

func (r failingReader) Read(_ []byte) (int, error) { return 0, r.err }

// A read error from the entropy source must propagate and never yield a
// partial code (SR-01, ADR-0002's "no option to fail open" posture applied to
// generation).
//
// This is an internal test, not an external one, because it substitutes
// randReader directly: crypto/rand.Read itself treats a read failure as fatal
// to the process as of Go 1.24, which would crash the test binary rather than
// let it observe the error — see randReader's doc comment in code.go.
func TestRandomGenerator_PropagatesRandReadError(t *testing.T) {
	broken := errors.New("boom")
	orig := randReader
	randReader = failingReader{err: broken}
	defer func() { randReader = orig }()

	g, err := NewRandomGenerator(AlphabetBase62, 10)
	if err != nil {
		t.Fatalf("NewRandomGenerator error = %v", err)
	}
	code, err := g.Generate(context.Background())
	if !errors.Is(err, broken) {
		t.Fatalf("Generate error = %v, want errors.Is(_, boom)", err)
	}
	if code != "" {
		t.Fatalf("Generate code = %q on error, want empty (no partial code)", code)
	}
}
