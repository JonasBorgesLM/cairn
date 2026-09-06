package policy

import (
	"net"

	"github.com/JonasBorgesLM/cairn"
)

// Default returns the policy a consumer gets by calling one function without
// having thought further about it, so it has to be the safe one.
//
// It chains BlockOwnDomains(ownDomains...) (SR-09) and BlockPrivateNetworks
// (SR-07), using net.DefaultResolver — exactly the two checks ADR-0005
// assigns to Policy rather than to ParseDestination, since both need a
// network or a configured domain list that a pure syntax check does not have.
// SR-05, SR-06, SR-08 and SR-10 are already enforced by ParseDestination
// before a Destination ever reaches a Policy; Default does not repeat them,
// and REQUIREMENTS.md's SR-05 through SR-10 are covered exactly once, between
// the two layers, not zero and not twice.
//
// BlockOwnDomains runs first because it is a string comparison against a
// fixed list, while BlockPrivateNetworks may need a DNS lookup: rejecting the
// service's own domain never needs to touch the network first, and Chain's
// Deny-beats-everything short-circuit means it usually will not.
//
// A consumer who wants a stricter scheme or length restriction composes
// SchemeAllowlist or MaxLength into their own policy.Chain alongside Default;
// those are optional tightening, not part of the safe floor.
func Default(ownDomains []string) cairn.Policy {
	return Chain(
		BlockOwnDomains(ownDomains...),
		BlockPrivateNetworks(net.DefaultResolver),
	)
}
