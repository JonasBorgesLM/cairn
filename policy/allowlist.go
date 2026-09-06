package policy

import (
	"context"

	"github.com/JonasBorgesLM/cairn"
)

// allowlist is a cairn.Policy classifying destinations against a fixed set of
// domains (FR-13).
type allowlist []string

// Allowlist returns a Policy that Allows a destination on one of domains (or
// a subdomain of one), and returns Interstitial — never Deny — for anything
// else, so a host can render ADR-0014's warning page for it without a second
// classification pass.
//
// An empty allowlist means everything is Interstitial. That is intentional:
// a policy with nothing to allow has nothing to Allow, and it is not a
// footgun, because Interstitial still lets the link through with a warning
// rather than rejecting it outright.
func Allowlist(domains ...string) cairn.Policy {
	return allowlist(domains)
}

// Evaluate implements cairn.Policy.
func (domains allowlist) Evaluate(_ context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	for _, domain := range domains {
		if hostMatchesDomain(d.Host(), domain) {
			return cairn.Allow, "", nil
		}
	}
	return cairn.Interstitial, cairn.ReasonNotAllowlisted, nil
}
