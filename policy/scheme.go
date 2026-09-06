package policy

import (
	"context"
	"strings"

	"github.com/JonasBorgesLM/cairn"
)

// schemeAllowlist is a cairn.Policy that only allows a configured set of
// schemes.
type schemeAllowlist map[string]bool

// SchemeAllowlist returns a Policy that denies any destination whose scheme
// is not in schemes (case-insensitive).
//
// ParseDestination already restricts every destination to http or https
// (SR-05); this is for a consumer who wants a stricter restriction still —
// https only, say — composed into their own policy chain.
func SchemeAllowlist(schemes ...string) cairn.Policy {
	allowed := make(schemeAllowlist, len(schemes))
	for _, s := range schemes {
		allowed[strings.ToLower(s)] = true
	}
	return allowed
}

// Evaluate implements cairn.Policy.
func (a schemeAllowlist) Evaluate(_ context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	if a[strings.ToLower(d.Scheme())] {
		return cairn.Allow, "", nil
	}
	return cairn.Deny, cairn.ReasonScheme, nil
}
