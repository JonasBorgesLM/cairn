package policy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/policy"
)

type stubPolicy struct {
	decision cairn.Decision
	reason   cairn.RejectReason
	err      error
}

func (p stubPolicy) Evaluate(_ context.Context, _ cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	return p.decision, p.reason, p.err
}

func allow() stubPolicy { return stubPolicy{decision: cairn.Allow} }
func deny(reason cairn.RejectReason) stubPolicy {
	return stubPolicy{decision: cairn.Deny, reason: reason}
}
func interstitial(reason cairn.RejectReason) stubPolicy {
	return stubPolicy{decision: cairn.Interstitial, reason: reason}
}

func mustDest(t *testing.T) cairn.Destination {
	t.Helper()
	d, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	return d
}

func TestChain_EmptyChainAllows(t *testing.T) {
	decision, _, err := policy.Chain().Evaluate(context.Background(), mustDest(t))
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestChain_AllAllowIsAllow(t *testing.T) {
	decision, _, err := policy.Chain(allow(), allow()).Evaluate(context.Background(), mustDest(t))
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Allow {
		t.Fatalf("decision = %v, want Allow", decision)
	}
}

func TestChain_DenyWinsRegardlessOfPosition(t *testing.T) {
	tests := []struct {
		name string
		ps   []cairn.Policy
	}{
		{"deny first", []cairn.Policy{deny(cairn.ReasonScheme), allow()}},
		{"deny last", []cairn.Policy{allow(), deny(cairn.ReasonScheme)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, reason, err := policy.Chain(tt.ps...).Evaluate(context.Background(), mustDest(t))
			if err != nil {
				t.Fatalf("Evaluate error = %v", err)
			}
			if decision != cairn.Deny {
				t.Fatalf("decision = %v, want Deny", decision)
			}
			if reason != cairn.ReasonScheme {
				t.Fatalf("reason = %q, want %q", reason, cairn.ReasonScheme)
			}
		})
	}
}

func TestChain_InterstitialWinsOverAllow(t *testing.T) {
	decision, reason, err := policy.Chain(allow(), interstitial(cairn.ReasonNotAllowlisted)).
		Evaluate(context.Background(), mustDest(t))
	if err != nil {
		t.Fatalf("Evaluate error = %v", err)
	}
	if decision != cairn.Interstitial {
		t.Fatalf("decision = %v, want Interstitial", decision)
	}
	if reason != cairn.ReasonNotAllowlisted {
		t.Fatalf("reason = %q, want %q", reason, cairn.ReasonNotAllowlisted)
	}
}

// This is the property ADR-0005 names: no ordering of policies can downgrade
// a rejection into a warning. Deny beats Interstitial regardless of which one
// a Chain's caller happened to list first.
func TestChain_DenyBeatsInterstitialRegardlessOfOrder(t *testing.T) {
	tests := []struct {
		name string
		ps   []cairn.Policy
	}{
		{"interstitial then deny", []cairn.Policy{interstitial(cairn.ReasonNotAllowlisted), deny(cairn.ReasonPrivateAddress)}},
		{"deny then interstitial", []cairn.Policy{deny(cairn.ReasonPrivateAddress), interstitial(cairn.ReasonNotAllowlisted)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, _, err := policy.Chain(tt.ps...).Evaluate(context.Background(), mustDest(t))
			if err != nil {
				t.Fatalf("Evaluate error = %v", err)
			}
			if decision != cairn.Deny {
				t.Fatalf("decision = %v, want Deny (never Interstitial)", decision)
			}
		})
	}
}

// The literal scenario from issue #20: chaining a real Allowlist (which
// answers Interstitial for a non-member) with a real BlockPrivateNetworks
// (which answers Deny for a private address) must Deny in both orders. A
// naive "first non-Allow wins" implementation that short-circuited on
// Allowlist's Interstitial would leak a private address as a mere warning.
func TestChain_AllowlistAndBlockPrivateBothOrdersDenyAPrivateAddress(t *testing.T) {
	allowlistPolicy := policy.Allowlist("example.com") // does not list the private address below
	blockPrivate := policy.BlockPrivateNetworks(fakeResolver{})

	tests := []struct {
		name string
		ps   []cairn.Policy
	}{
		{"allowlist then block-private", []cairn.Policy{allowlistPolicy, blockPrivate}},
		{"block-private then allowlist", []cairn.Policy{blockPrivate, allowlistPolicy}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, reason := evaluate(t, policy.Chain(tt.ps...), "https://127.0.0.1/")
			if decision != cairn.Deny {
				t.Fatalf("decision = %v, want Deny (never Interstitial)", decision)
			}
			if reason != cairn.ReasonPrivateAddress {
				t.Fatalf("reason = %q, want %q", reason, cairn.ReasonPrivateAddress)
			}
		})
	}
}

func TestChain_PropagatesAnErrorFromAnySubPolicy(t *testing.T) {
	boom := errors.New("boom")
	_, _, err := policy.Chain(allow(), stubPolicy{err: boom}, allow()).
		Evaluate(context.Background(), mustDest(t))
	if !errors.Is(err, boom) {
		t.Fatalf("Evaluate error = %v, want errors.Is(_, boom)", err)
	}
}
