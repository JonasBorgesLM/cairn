package cairn_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// The worked example from ADR-0002: one million live links, an attacker
// sustaining 10,000 requests/second for thirty days. L=7 and L=8 are
// self-evidently unsafe there; L=10 (the default) and L=12 are safe.
func TestCheckKeyspaceDensity_MatchesADR0002WorkedExample(t *testing.T) {
	tests := []struct {
		length    int
		wantError bool
	}{
		{7, true},
		{8, true},
		{10, false},
		{12, false},
	}

	for _, tt := range tests {
		t.Run(strings.Repeat("x", tt.length), func(t *testing.T) {
			err := cairn.CheckKeyspaceDensity(cairn.AlphabetBase62, tt.length, 1_000_000)
			if tt.wantError && err == nil {
				t.Fatalf("length=%d: error = nil, want non-nil", tt.length)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("length=%d: error = %v, want nil", tt.length, err)
			}
		})
	}
}

// The literal scenario from the issue: a consumer choosing a short code for
// prettier links against a realistic expected link count must be stopped at
// construction, not merely warned.
func TestCheckKeyspaceDensity_RejectsShortCodeWithManyExpectedLinks(t *testing.T) {
	err := cairn.CheckKeyspaceDensity(cairn.AlphabetBase62, 5, 1_000_000)
	if err == nil {
		t.Fatalf("CheckKeyspaceDensity(length=5, n=1e6) error = nil, want non-nil")
	}
}

// The error must carry the computed number, not just a verdict, so a host can
// see the arithmetic that produced the rejection.
func TestCheckKeyspaceDensity_ErrorContainsTheComputedNumber(t *testing.T) {
	err := cairn.CheckKeyspaceDensity(cairn.AlphabetBase62, 5, 1_000_000)
	if err == nil {
		t.Fatalf("error = nil, want non-nil")
	}

	// E[hits] = R*N/A^L, computed the same way CheckKeyspaceDensity does, so
	// this test breaks if the arithmetic it depends on ever changes silently.
	const attackerRequests = 10_000 * 86400 * 30 // ADR-0002's worked assumption
	wantHits := attackerRequests * 1_000_000 / math.Pow(62, 5)
	wantSubstring := fmt.Sprintf("%.3g", wantHits)

	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("error = %q, want it to contain the computed E[hits] %q", err.Error(), wantSubstring)
	}
}

func TestCheckKeyspaceDensity_ZeroExpectedLinksSkipsTheCheck(t *testing.T) {
	// WithExpectedLinks is optional; 0 means "unspecified", not "zero links",
	// and there is nothing to check without a declared expectation.
	if err := cairn.CheckKeyspaceDensity(cairn.AlphabetBase62, 5, 0); err != nil {
		t.Fatalf("CheckKeyspaceDensity(n=0) error = %v, want nil", err)
	}
}

func TestCheckKeyspaceDensity_RejectsNonPositiveLength(t *testing.T) {
	if err := cairn.CheckKeyspaceDensity(cairn.AlphabetBase62, 0, 1000); err == nil {
		t.Fatalf("CheckKeyspaceDensity(length=0) error = nil, want non-nil")
	}
}
