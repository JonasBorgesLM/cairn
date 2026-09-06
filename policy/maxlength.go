package policy

import (
	"context"

	"github.com/JonasBorgesLM/cairn"
)

// maxLength is a cairn.Policy enforcing a maximum destination length.
type maxLength int

// MaxLength returns a Policy that denies any destination longer than n bytes.
//
// ParseDestination already enforces a conservative default maximum of 2000
// bytes (SR-08); this is for a consumer who wants a stricter bound of their
// own, composed into their own policy chain.
func MaxLength(n int) cairn.Policy {
	return maxLength(n)
}

// Evaluate implements cairn.Policy.
func (n maxLength) Evaluate(_ context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	if len(d.Raw()) > int(n) {
		return cairn.Deny, cairn.ReasonTooLong, nil
	}
	return cairn.Allow, "", nil
}
