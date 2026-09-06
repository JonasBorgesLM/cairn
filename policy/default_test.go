package policy_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

// Default must Deny a private address (SR-07): the network-dependent check
// ParseDestination structurally cannot perform.
func TestDefault_DeniesAPrivateAddress(t *testing.T) {
	p := policy.Default([]string{"example.com"})
	decision, reason := evaluate(t, p, "https://127.0.0.1/")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonPrivateAddress {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonPrivateAddress)
	}
}

// Default must Deny the service's own domain (SR-09).
//
// Negative control: this test was run against a build of ownDomains.Evaluate
// that always returned Allow. It failed -- the request fell through to
// BlockPrivateNetworks and got denied for the wrong reason
// (host_resolution_failed) rather than own_domain. Restored immediately
// after.
func TestDefault_DeniesTheServicesOwnDomain(t *testing.T) {
	p := policy.Default([]string{"short.example"})
	decision, reason := evaluate(t, p, "https://short.example/anything")
	if decision != cairn.Deny {
		t.Fatalf("decision = %v, want Deny", decision)
	}
	if reason != cairn.ReasonOwnDomain {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonOwnDomain)
	}
}

// Default must Allow an ordinary public destination that is neither private
// nor the service's own domain.
func TestDefault_AllowsAnOrdinaryPublicDestination(t *testing.T) {
	p := policy.Default([]string{"short.example"})
	decision, _ := evaluate(t, p, "https://93.184.216.34/")
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

// This is the audit test issue #25 asks for: the composed set of checks
// across ParseDestination (core) and policy.Default together must cover
// SR-05 through SR-10 exactly, per ADR-0005's stated division -- syntax in
// the core, semantics (SR-07, SR-09) in Default -- so that dropping any one
// of them, from either layer, fails this test.
func TestSR05ThroughSR10AreAllCoveredByParseDestinationOrDefault(t *testing.T) {
	ownDomains := []string{"short.example"}
	p := policy.Default(ownDomains)

	tests := []struct {
		name       string
		raw        string
		wantReason cairn.RejectReason
		// layer is "parse" if ParseDestination itself must reject raw, or
		// "policy" if ParseDestination must accept it and p.Evaluate must
		// then reject it.
		layer string
	}{
		{"SR-05 scheme allowlist", "javascript:alert(1)", cairn.ReasonScheme, "parse"},
		{"SR-06 no userinfo", "https://user@evil.example/", cairn.ReasonUserinfo, "parse"},
		{"SR-07 no internal destinations", "https://127.0.0.1/", cairn.ReasonPrivateAddress, "policy"},
		{"SR-08 length limit", "https://example.com/" + strings.Repeat("a", 2001), cairn.ReasonTooLong, "parse"},
		{"SR-09 anti-loop", "https://short.example/x", cairn.ReasonOwnDomain, "policy"},
		{"SR-10 no control characters", "https://example.com/\r\n", cairn.ReasonControlChars, "parse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := cairn.ParseDestination(tt.raw)
			switch tt.layer {
			case "parse":
				if err == nil {
					t.Fatalf("ParseDestination(%q) error = nil, want a rejection for %s", tt.raw, tt.name)
				}
				reason, ok := cairn.RejectReasonFrom(err)
				if !ok || reason != tt.wantReason {
					t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, tt.wantReason)
				}
			case "policy":
				if err != nil {
					t.Fatalf("ParseDestination(%q) error = %v, want nil (this SR is Default's job)", tt.raw, err)
				}
				decision, reason, evalErr := p.Evaluate(t.Context(), d)
				if evalErr != nil {
					t.Fatalf("Evaluate error = %v", evalErr)
				}
				if decision != cairn.Deny {
					t.Fatalf("decision = %v, want Deny for %s", decision, tt.name)
				}
				if reason != tt.wantReason {
					t.Fatalf("reason = %q, want %q", reason, tt.wantReason)
				}
			default:
				t.Fatalf("unknown layer %q", tt.layer)
			}
		})
	}
}

func ExampleDefault() {
	p := policy.Default([]string{"short.example"})

	d, err := cairn.ParseDestination("https://93.184.216.34/")
	if err != nil {
		panic(err)
	}

	decision, _, err := p.Evaluate(context.Background(), d)
	if err != nil {
		panic(err)
	}

	fmt.Println(decision == cairn.Allow)

	// Output: true
}
