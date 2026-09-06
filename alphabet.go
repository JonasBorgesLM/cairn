package cairn

import (
	"fmt"
	"math"
)

// Code is a short code. It is untrusted input on the resolve path until
// [Alphabet.Validate] has accepted it (SR-21).
type Code string

// String implements fmt.Stringer.
func (c Code) String() string {
	return string(c)
}

// maxCodeLength is a defensive sanity bound on a code's length, independent of
// any particular generator's configured length. It exists so that
// Alphabet.Validate can reject a pathologically long code — one large enough
// to be a resource-exhaustion attempt rather than a real short code — even
// before a Shortener exists to enforce its own, tighter, configured length.
// The per-configuration bound (WithCodeLength, WithVanity) is enforced by
// Shortener at construction and belongs to it, not here.
const maxCodeLength = 128

// urlPathSafeRunes is the set an Alphabet's runes must be drawn from: anything
// outside it needs percent-encoding in a URL path segment, which would make
// the code's own rendering ambiguous and defeats SR-21's premise that a code
// is validatable before it becomes a key (ADR-0004).
const urlPathSafeRunes = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

// Alphabet is a validated set of runes usable in a [Code]. Construct one with
// [NewAlphabet], or use the provided [AlphabetBase62] and [AlphabetUnambiguous].
type Alphabet struct {
	runes         string
	set           map[rune]struct{}
	defaultLength int
}

// NewAlphabet builds an Alphabet from runes. It rejects duplicates, non-ASCII
// runes, and anything outside [A-Za-z0-9_-] — the URL-path-safe set — as well
// as an empty set (ADR-0004).
//
// The returned Alphabet's [Alphabet.DefaultLength] is computed so that its
// keyspace (bits per rune × length) is at least [AlphabetBase62]'s default
// keyspace, so a custom alphabet never quietly trades away ADR-0002's entropy
// floor.
func NewAlphabet(runes string) (Alphabet, error) {
	if runes == "" {
		return Alphabet{}, fmt.Errorf("cairn: alphabet must not be empty")
	}

	set := make(map[rune]struct{}, len(runes))
	for _, r := range runes {
		if r > 127 {
			return Alphabet{}, fmt.Errorf("cairn: alphabet rune %q is not ASCII", r)
		}
		if !isURLPathSafe(r) {
			return Alphabet{}, fmt.Errorf("cairn: alphabet rune %q is outside [A-Za-z0-9_-]", r)
		}
		if _, dup := set[r]; dup {
			return Alphabet{}, fmt.Errorf("cairn: alphabet rune %q is duplicated", r)
		}
		set[r] = struct{}{}
	}

	a := Alphabet{runes: runes, set: set}
	a.defaultLength = defaultLengthFor(a.BitsPerRune())
	return a, nil
}

func isURLPathSafe(r rune) bool {
	for _, safe := range urlPathSafeRunes {
		if r == safe {
			return true
		}
	}
	return false
}

// base62Size is AlphabetBase62's rune count, used below to compute the
// entropy floor without depending on AlphabetBase62 itself — which would
// create an initialization cycle, since constructing it calls defaultLengthFor.
const base62Size = 62

// base62DefaultBits is AlphabetBase62's default keyspace in bits
// (bits/rune × length = 5.954 × 10 ≈ 59.5), the entropy floor every other
// alphabet's computed default length must meet or exceed (ADR-0002, ADR-0004).
var base62DefaultBits = math.Log2(base62Size) * 10

// defaultLengthFor returns the shortest length whose keyspace, at bitsPerRune,
// is at least base62's default keyspace.
func defaultLengthFor(bitsPerRune float64) int {
	return int(math.Ceil(base62DefaultBits / bitsPerRune))
}

func mustAlphabet(runes string) Alphabet {
	a, err := NewAlphabet(runes)
	if err != nil {
		panic("cairn: invalid built-in alphabet: " + err.Error())
	}
	return a
}

// AlphabetBase62 is the default alphabet: [0-9A-Za-z], 5.954 bits per rune,
// with a default code length of 10 (ADR-0002, ADR-0004).
var AlphabetBase62 = mustAlphabet("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

// AlphabetUnambiguous omits the six runes humans confuse when transcribing —
// 0 O o 1 l I — at 5.807 bits per rune. Its default code length is 11, one
// more than AlphabetBase62's, so that removing a class of human error never
// costs any of ADR-0002's security margin (ADR-0004).
var AlphabetUnambiguous = mustAlphabet(withoutRunes("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", "0Oo1lI"))

func withoutRunes(runes, drop string) string {
	dropSet := make(map[rune]struct{}, len(drop))
	for _, r := range drop {
		dropSet[r] = struct{}{}
	}
	out := make([]rune, 0, len(runes))
	for _, r := range runes {
		if _, excluded := dropSet[r]; excluded {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// Size returns the number of runes in the alphabet.
func (a Alphabet) Size() int {
	return len(a.set)
}

// BitsPerRune returns log2(Size()), the entropy contributed by each rune of a
// code drawn uniformly from this alphabet.
func (a Alphabet) BitsPerRune() float64 {
	return math.Log2(float64(a.Size()))
}

// DefaultLength returns the code length this alphabet recommends: the
// shortest length whose keyspace is at least AlphabetBase62's default
// keyspace. WithCodeLength overrides it explicitly; that override still goes
// through the density check of ADR-0002 (ADR-0004).
func (a Alphabet) DefaultLength() int {
	return a.defaultLength
}

// Contains reports whether r is in the alphabet.
func (a Alphabet) Contains(r rune) bool {
	_, ok := a.set[r]
	return ok
}

// Validate reports whether every rune of c is in the alphabet, and that c is
// neither empty nor implausibly long. It is called before a code is
// concatenated into a store key, never after (SR-21).
func (a Alphabet) Validate(c Code) error {
	if len(c) == 0 {
		return fmt.Errorf("%w: code is empty", ErrInvalidCode)
	}
	if len(c) > maxCodeLength {
		return fmt.Errorf("%w: code exceeds %d bytes", ErrInvalidCode, maxCodeLength)
	}
	for _, r := range string(c) {
		if !a.Contains(r) {
			return fmt.Errorf("%w: rune %q is outside the alphabet", ErrInvalidCode, r)
		}
	}
	return nil
}
