package cairn

import (
	"fmt"
	"math"
)

// Assumed attacker campaign used by [CheckKeyspaceDensity]: sustained
// scanning at assumedAttackerReqPerSec for assumedAttackerDays, the same
// worked assumption ADR-0002 uses (10,000 req/s for 30 days, ≈2.6×10^10
// requests). It is not configurable: the check is a floor against a
// realistic scanner, not a tunable that could be loosened away.
const (
	assumedAttackerReqPerSec = 10_000
	assumedAttackerDays      = 30
)

// unsafeExpectedHits is the E[hits] value at or above which a configuration
// is refused: an attacker expected, on average, to find at least one live
// link during the assumed campaign. cairn's own default (base62, length 10)
// sits roughly 30x below it against a million expected links.
const unsafeExpectedHits = 1.0

// CheckKeyspaceDensity reports an error if a code of the given length, drawn
// from alphabet, cannot safely support expectedLinks live links against a
// scanning attacker (SR-02, T-10, ADR-0002).
//
// Scan resistance is a property of density, not of the code source: for an
// alphabet size A, code length L, N live links and an attacker issuing R
// requests,
//
//	E[hits] = R * N / A^L
//
// CheckKeyspaceDensity evaluates this formula with R fixed at the campaign
// above and rejects a configuration whose E[hits] is at least 1 — an attacker
// expected to succeed at least once. A host with a larger or smaller N than
// the example ADR-0002 works through computes their own required L from the
// same formula; this function is that computation, refusing to let a
// configuration through it disagrees with.
//
// expectedLinks of 0 means "unspecified" and skips the check: a host that has
// not called WithExpectedLinks has not declared an N to check against, which
// is different from declaring zero live links.
//
// [Shortener.New] (M4) applies this to the configured Alphabet, code length
// and WithExpectedLinks value; a consumer does not normally call it directly.
func CheckKeyspaceDensity(alphabet Alphabet, length int, expectedLinks int64) error {
	if length <= 0 {
		return fmt.Errorf("cairn: code length must be positive, got %d", length)
	}
	if expectedLinks <= 0 {
		return nil
	}

	const attackerRequests = float64(assumedAttackerReqPerSec) * 86400 * assumedAttackerDays

	keyspace := math.Pow(float64(alphabet.Size()), float64(length))
	expectedHits := attackerRequests * float64(expectedLinks) / keyspace

	if expectedHits >= unsafeExpectedHits {
		return fmt.Errorf(
			"cairn: alphabet size %d and code length %d give a keyspace of %.3g codes; "+
				"against %d expected live links and an attacker sustaining %d req/s for %d days "+
				"(%.3g requests), the expected number of successful guesses is %.3g, which is not "+
				"self-evidently safe (SR-02, ADR-0002); increase the code length, use a larger "+
				"alphabet, or lower the declared expected link count",
			alphabet.Size(), length, keyspace,
			expectedLinks, assumedAttackerReqPerSec, assumedAttackerDays, attackerRequests,
			expectedHits,
		)
	}
	return nil
}
