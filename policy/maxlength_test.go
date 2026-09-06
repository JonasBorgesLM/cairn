package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

func TestMaxLength_AllowsWithinLimit(t *testing.T) {
	p := policy.MaxLength(100)
	d, err := cairn.ParseDestination("https://example.com/short")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	decision, _, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestMaxLength_DeniesOverTheStricterLimit(t *testing.T) {
	// MaxLength(50) is stricter than ParseDestination's own 2000-byte default
	// (SR-08); this is what lets a consumer impose their own tighter bound.
	raw := "https://example.com/" + strings.Repeat("a", 100)
	p := policy.MaxLength(50)
	d, err := cairn.ParseDestination(raw)
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	decision, reason, err := p.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonTooLong {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonTooLong)
	}
}
