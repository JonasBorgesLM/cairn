package policy

import (
	"context"

	"github.com/JonasBorgesLM/cairn"
)

// ownDomains is a cairn.Policy denying the host's own configured domains
// (SR-09).
type ownDomains []string

// BlockOwnDomains returns a Policy that denies a destination on any of
// domains, or a subdomain of one.
//
// This covers only the host's own domains. Chained third-party shorteners are
// explicitly not addressed: maintaining a denylist of every other shortener
// domain is a losing game, and this godoc says so rather than implying
// coverage it does not have.
func BlockOwnDomains(domains ...string) cairn.Policy {
	return ownDomains(domains)
}

// Evaluate implements cairn.Policy.
func (domains ownDomains) Evaluate(_ context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	for _, domain := range domains {
		if hostMatchesDomain(d.Host(), domain) {
			return cairn.Deny, cairn.ReasonOwnDomain, nil
		}
	}
	return cairn.Allow, "", nil
}
