package policy

import (
	"context"

	"github.com/JonasBorgesLM/cairn"
)

// chain evaluates every policy in ps and combines their decisions by
// severity, not by position.
type chain []cairn.Policy

// Chain composes ps into a single Policy. Every policy is evaluated — a Chain
// does not stop at the first non-Allow result — and the most severe decision
// wins: Deny beats Interstitial beats Allow. This is what ADR-0005 means by
// "no ordering of policies can downgrade a rejection into a warning":
// Chain(Allowlist(...), BlockPrivateNetworks(...)) and the reverse both Deny a
// private address that is also outside the allowlist, because the Deny is
// never allowed to lose to an Interstitial found elsewhere in the chain.
//
// An empty Chain allows everything — there is nothing in it to object.
//
// If any policy returns a non-nil error, Chain stops and returns that error
// immediately, fail-closed (SR-20): an internal failure is never silently
// treated as permission.
func Chain(ps ...cairn.Policy) cairn.Policy {
	return chain(ps)
}

// Evaluate implements cairn.Policy.
func (c chain) Evaluate(ctx context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	best := cairn.Allow
	var bestReason cairn.RejectReason

	for _, p := range c {
		decision, reason, err := p.Evaluate(ctx, d)
		if err != nil {
			return cairn.Deny, "", err
		}
		switch decision {
		case cairn.Deny:
			// Nothing outranks Deny; stop here.
			return cairn.Deny, reason, nil
		case cairn.Interstitial:
			if best == cairn.Allow {
				best, bestReason = cairn.Interstitial, reason
			}
		case cairn.Allow:
			// Does not change the running best.
		}
	}

	return best, bestReason, nil
}
