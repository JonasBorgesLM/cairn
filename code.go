package cairn

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
)

// CodeGenerator produces codes.
//
// An implementation MUST draw from a cryptographically secure source. A
// counter, a timestamp, a hash of the destination, or a math/rand generator
// all reintroduce T-02 — the last one while looking random, since a hash
// looks unpredictable while being a deterministic function of a guessable
// input. This is the single most dangerous interface in the library to
// implement casually: get it wrong, and every code cairn issues is
// enumerable by an attacker who has seen a handful of them (ADR-0002).
//
// Prefer [NewRandomGenerator] unless you have a specific reason not to.
type CodeGenerator interface {
	Generate(ctx context.Context) (Code, error)
}

// randReader is the entropy source, indirected so an internal test can
// substitute a failing Reader to verify that a read error propagates rather
// than yielding a partial code. It is read through io.ReadFull directly,
// rather than through crypto/rand.Read, because as of Go 1.24 crypto/rand.Read
// treats a read failure as fatal to the process instead of returning an
// error — appropriate for its own use (see crypto/rand's documentation), but
// it would make that failure mode untestable and would crash a host process
// on a condition this package would otherwise simply report.
var randReader io.Reader = rand.Reader

// randomGenerator is the default CodeGenerator: rejection sampling over
// crypto/rand.
type randomGenerator struct {
	alphabet []rune
	length   int
}

// NewRandomGenerator returns the default generator: rejection sampling over
// crypto/rand, so every rune of a is equally likely. Rejection sampling, not
// modulo reduction — b % len(a) over a uniform byte biases the low runes of
// the alphabet, since 256 is rarely a multiple of the alphabet size, which
// shrinks the effective keyspace for free and for no reason (SR-01,
// ADR-0002).
func NewRandomGenerator(a Alphabet, length int) (CodeGenerator, error) {
	if length <= 0 {
		return nil, fmt.Errorf("cairn: code length must be positive, got %d", length)
	}
	return &randomGenerator{alphabet: []rune(a.runes), length: length}, nil
}

// Generate returns a new code drawn uniformly from g's alphabet. On a
// crypto/rand read error it returns that error and an empty Code — never a
// code assembled from fewer than the full length of accepted draws.
func (g *randomGenerator) Generate(ctx context.Context) (Code, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	n := len(g.alphabet)
	// limit is the largest multiple of n that fits in a byte. A drawn byte
	// at or above it is discarded and redrawn rather than reduced, which is
	// what keeps every rune equally likely regardless of how n divides 256.
	limit := (256 / n) * n

	runes := make([]rune, 0, g.length)
	// buf is sized generously so that, for realistic alphabet sizes, one read
	// almost always supplies the whole code even after rejections; Read is
	// called again only in the rare case it does not.
	buf := make([]byte, g.length*2+8)

	for len(runes) < g.length {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := io.ReadFull(randReader, buf); err != nil {
			return "", fmt.Errorf("cairn: reading random bytes: %w", err)
		}
		for _, b := range buf {
			if len(runes) == g.length {
				break
			}
			if int(b) >= limit {
				continue
			}
			runes = append(runes, g.alphabet[int(b)%n])
		}
	}

	return Code(runes), nil
}
