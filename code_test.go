package cairn_test

import (
	"context"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/JonasBorgesLM/cairn"
)

func TestNewRandomGenerator_RejectsNonPositiveLength(t *testing.T) {
	tests := []int{0, -1}
	for _, length := range tests {
		if _, err := cairn.NewRandomGenerator(cairn.AlphabetBase62, length); err == nil {
			t.Fatalf("NewRandomGenerator(length=%d) error = nil, want non-nil", length)
		}
	}
}

func TestRandomGenerator_GeneratesCodeOfTheRequestedLength(t *testing.T) {
	g, err := cairn.NewRandomGenerator(cairn.AlphabetBase62, 10)
	if err != nil {
		t.Fatalf("NewRandomGenerator error = %v", err)
	}
	code, err := g.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if len(code) != 10 {
		t.Fatalf("len(code) = %d, want 10", len(code))
	}
}

func TestRandomGenerator_GeneratesOnlyAlphabetRunes(t *testing.T) {
	g, err := cairn.NewRandomGenerator(cairn.AlphabetUnambiguous, 20)
	if err != nil {
		t.Fatalf("NewRandomGenerator error = %v", err)
	}
	code, err := g.Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate error = %v", err)
	}
	if err := cairn.AlphabetUnambiguous.Validate(code); err != nil {
		t.Fatalf("generated code %q failed Validate: %v", code, err)
	}
}

func TestRandomGenerator_GeneratesDifferentCodes(t *testing.T) {
	g, err := cairn.NewRandomGenerator(cairn.AlphabetBase62, 10)
	if err != nil {
		t.Fatalf("NewRandomGenerator error = %v", err)
	}
	seen := map[cairn.Code]bool{}
	for range 50 {
		code, err := g.Generate(context.Background())
		if err != nil {
			t.Fatalf("Generate error = %v", err)
		}
		if seen[code] {
			t.Fatalf("Generate produced the same code twice in 50 draws: %q", code)
		}
		seen[code] = true
	}
}

// Rejection sampling over crypto/rand must not bias any rune of the alphabet
// (SR-01, ADR-0002): b % 62 over a uniform byte favours the first eight runes,
// since 256 is not a multiple of 62.
//
// Negative control: this exact test was run against randomGenerator.Generate
// with its rejection check disabled (`b % n` applied unconditionally, no
// discard above the multiple-of-n limit). It failed with a chi-square
// statistic of 459.50, well above the threshold below. The check was then
// restored.
func TestNewRandomGenerator_ChiSquareUniformity(t *testing.T) {
	a := cairn.AlphabetBase62
	g, err := cairn.NewRandomGenerator(a, 1)
	if err != nil {
		t.Fatalf("NewRandomGenerator error = %v", err)
	}

	const samplesPerRune = 1000
	n := a.Size()
	samples := n * samplesPerRune

	counts := make(map[rune]int, n)
	for range samples {
		code, err := g.Generate(context.Background())
		if err != nil {
			t.Fatalf("Generate error = %v", err)
		}
		r, _ := utf8.DecodeRuneInString(string(code))
		counts[r]++
	}

	if len(counts) != n {
		t.Fatalf("observed %d distinct runes in %d samples, want all %d of the alphabet", len(counts), samples, n)
	}

	expected := float64(samples) / float64(n)
	var chiSquare float64
	for _, c := range counts {
		diff := float64(c) - expected
		chiSquare += diff * diff / expected
	}

	// df = n-1 = 61. Mean of a true chi-square(61) is 61; this threshold is
	// generous (mean + roughly 8 standard deviations) to keep the test from
	// ever flaking against a genuinely uniform source, while a modulo-biased
	// generator's statistic lands in the many hundreds at this sample size
	// (see the negative control above).
	const threshold = 200.0
	if chiSquare > threshold {
		t.Fatalf("chi-square statistic = %.2f, want <= %.2f (rune counts: %v)", chiSquare, threshold, counts)
	}
}

func ExampleNewRandomGenerator() {
	g, err := cairn.NewRandomGenerator(cairn.AlphabetBase62, 10)
	if err != nil {
		panic(err)
	}

	code, err := g.Generate(context.Background())
	if err != nil {
		panic(err)
	}

	fmt.Println(len(code))

	// Output: 10
}
