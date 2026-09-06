package cairn_test

import (
	"context"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// Decision's ordering is load-bearing: policy.Chain's "Deny beats
// Interstitial" precedence and the exhaustive-switch linter both depend on
// these three values staying Deny, Interstitial, Allow in that order.
func TestDecision_Values(t *testing.T) {
	if cairn.Deny != 0 {
		t.Fatalf("Deny = %d, want 0", cairn.Deny)
	}
	if cairn.Interstitial != 1 {
		t.Fatalf("Interstitial = %d, want 1", cairn.Interstitial)
	}
	if cairn.Allow != 2 {
		t.Fatalf("Allow = %d, want 2", cairn.Allow)
	}
}

// A minimal Policy implementation must satisfy the interface with exactly
// this signature, pinning it against an accidental breaking change.
type stubPolicy struct {
	decision cairn.Decision
	reason   cairn.RejectReason
}

func (p stubPolicy) Evaluate(_ context.Context, _ cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	return p.decision, p.reason, nil
}

func TestPolicy_Interface(t *testing.T) {
	var p cairn.Policy = stubPolicy{decision: cairn.Allow}
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	decision, _, err := p.Evaluate(context.Background(), dest)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("Evaluate decision = %v, want Allow", decision)
	}
}
