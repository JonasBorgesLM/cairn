package cairn_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func ExampleAlphabet_Validate() {
	code := cairn.Code("abc:def")

	err := cairn.AlphabetBase62.Validate(code)
	fmt.Println(err)

	// Output: cairn: invalid code: rune ':' is outside the alphabet
}

func TestNewAlphabet_RejectsDuplicates(t *testing.T) {
	_, err := cairn.NewAlphabet("aab")
	if err == nil {
		t.Fatalf("NewAlphabet(\"aab\") error = nil, want non-nil")
	}
}

func TestNewAlphabet_RejectsNonASCII(t *testing.T) {
	_, err := cairn.NewAlphabet("abé")
	if err == nil {
		t.Fatalf("NewAlphabet with non-ASCII rune error = nil, want non-nil")
	}
}

func TestNewAlphabet_RejectsRunesOutsideURLPathSafeSet(t *testing.T) {
	tests := []string{"a b", "a/b", "a:b", "a?b", "a#b", "a.b", "a+b"}
	for _, runes := range tests {
		t.Run(runes, func(t *testing.T) {
			if _, err := cairn.NewAlphabet(runes); err == nil {
				t.Fatalf("NewAlphabet(%q) error = nil, want non-nil", runes)
			}
		})
	}
}

func TestNewAlphabet_AcceptsURLPathSafeSet(t *testing.T) {
	a, err := cairn.NewAlphabet("abc_-XYZ019")
	if err != nil {
		t.Fatalf("NewAlphabet error = %v, want nil", err)
	}
	if a.Size() != 11 {
		t.Fatalf("Size() = %d, want 11", a.Size())
	}
}

func TestNewAlphabet_RejectsEmpty(t *testing.T) {
	if _, err := cairn.NewAlphabet(""); err == nil {
		t.Fatalf("NewAlphabet(\"\") error = nil, want non-nil")
	}
}

func TestAlphabet_Contains(t *testing.T) {
	a, err := cairn.NewAlphabet("abc")
	if err != nil {
		t.Fatalf("NewAlphabet error = %v", err)
	}
	if !a.Contains('a') {
		t.Fatalf("Contains('a') = false, want true")
	}
	if a.Contains('z') {
		t.Fatalf("Contains('z') = true, want false")
	}
}

func TestAlphabetBase62_Size(t *testing.T) {
	if got := cairn.AlphabetBase62.Size(); got != 62 {
		t.Fatalf("AlphabetBase62.Size() = %d, want 62", got)
	}
}

func TestAlphabetUnambiguous_Size(t *testing.T) {
	if got := cairn.AlphabetUnambiguous.Size(); got != 56 {
		t.Fatalf("AlphabetUnambiguous.Size() = %d, want 56", got)
	}
}

func TestAlphabetUnambiguous_OmitsConfusableRunes(t *testing.T) {
	for _, r := range []rune{'0', 'O', 'o', '1', 'l', 'I'} {
		if cairn.AlphabetUnambiguous.Contains(r) {
			t.Fatalf("AlphabetUnambiguous.Contains(%q) = true, want false", r)
		}
	}
}

// BitsPerRune is tested against the documented values (ADR-0004).
func TestAlphabet_BitsPerRune(t *testing.T) {
	tests := []struct {
		name string
		a    cairn.Alphabet
		want float64
	}{
		{"base62", cairn.AlphabetBase62, 5.954},
		{"unambiguous", cairn.AlphabetUnambiguous, 5.807},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.BitsPerRune()
			if math.Abs(got-tt.want) > 0.001 {
				t.Fatalf("BitsPerRune() = %v, want ~%v", got, tt.want)
			}
		})
	}
}

func TestAlphabet_Validate_AcceptsCodeWithinAlphabet(t *testing.T) {
	if err := cairn.AlphabetBase62.Validate(cairn.Code("Ab3XYz9")); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestAlphabet_Validate_RejectsRuneOutsideAlphabet(t *testing.T) {
	a, err := cairn.NewAlphabet("abc")
	if err != nil {
		t.Fatalf("NewAlphabet error = %v", err)
	}

	tests := []struct {
		name string
		code cairn.Code
	}{
		{"colon", cairn.Code("a:b")},
		{"asterisk", cairn.Code("a*b")},
		{"newline", cairn.Code("a\nb")},
		{"NUL byte", cairn.Code("a\x00b")},
		{"rune not in this alphabet", cairn.Code("xyz")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := a.Validate(tt.code); err == nil {
				t.Fatalf("Validate(%q) error = nil, want non-nil", tt.code)
			}
		})
	}
}

func TestAlphabet_Validate_RejectsEmptyCode(t *testing.T) {
	if err := cairn.AlphabetBase62.Validate(cairn.Code("")); err == nil {
		t.Fatalf("Validate(\"\") error = nil, want non-nil")
	}
}

func TestAlphabet_Validate_RejectsOverLongCode(t *testing.T) {
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	a, err := cairn.NewAlphabet("a")
	if err != nil {
		t.Fatalf("NewAlphabet error = %v", err)
	}
	if err := a.Validate(cairn.Code(long)); err == nil {
		t.Fatalf("Validate(129-rune code) error = nil, want non-nil")
	}
}

func TestAlphabet_Validate_ReturnsErrInvalidCode(t *testing.T) {
	if err := cairn.AlphabetBase62.Validate(cairn.Code("a:b")); !errors.Is(err, cairn.ErrInvalidCode) {
		t.Fatalf("Validate error = %v, want errors.Is(_, ErrInvalidCode)", err)
	}
}

func TestCode_String(t *testing.T) {
	if got := cairn.Code("abc123").String(); got != "abc123" {
		t.Fatalf("String() = %q, want %q", got, "abc123")
	}
}

// AlphabetUnambiguous defaults its code length to 11, one more than base62's
// 10, so choosing legibility never quietly trades away ADR-0002's entropy
// floor.
func TestAlphabetUnambiguous_DefaultLength(t *testing.T) {
	if got := cairn.AlphabetUnambiguous.DefaultLength(); got != 11 {
		t.Fatalf("AlphabetUnambiguous.DefaultLength() = %d, want 11", got)
	}
}

func TestAlphabetBase62_DefaultLength(t *testing.T) {
	if got := cairn.AlphabetBase62.DefaultLength(); got != 10 {
		t.Fatalf("AlphabetBase62.DefaultLength() = %d, want 10", got)
	}
}

// A custom alphabet's DefaultLength is computed so its keyspace is at least
// base62's default (62^10), preserving the entropy floor for any alphabet
// size, not only the two that ship (ADR-0004).
func TestNewAlphabet_DefaultLengthPreservesEntropyFloor(t *testing.T) {
	a, err := cairn.NewAlphabet("01") // 1 bit/rune
	if err != nil {
		t.Fatalf("NewAlphabet error = %v", err)
	}
	floorBits := cairn.AlphabetBase62.BitsPerRune() * float64(cairn.AlphabetBase62.DefaultLength())
	gotBits := a.BitsPerRune() * float64(a.DefaultLength())
	if gotBits < floorBits {
		t.Fatalf("keyspace = %v bits, want >= %v bits (base62 default)", gotBits, floorBits)
	}
}
