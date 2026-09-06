package cairn

import (
	"errors"
	"fmt"
)

// Sentinel errors, all comparable with errors.Is (NFR-07).
var (
	// ErrCodeExists reports that a code is already taken. Save returns it on a
	// conditional-write collision (SR-18).
	ErrCodeExists = errors.New("cairn: code already exists")

	// ErrCodeNotFound reports that a code has no record — including one that
	// was never issued and one whose grace window has elapsed (ADR-0009).
	ErrCodeNotFound = errors.New("cairn: code not found")

	// ErrLinkExpired reports that a code resolved to a record whose logical
	// expiry has passed. It is distinguishable from ErrCodeNotFound so a host
	// can render a different message (FR-09).
	ErrLinkExpired = errors.New("cairn: link expired")

	// ErrLinkRevoked reports that a code resolved to a record that was
	// explicitly revoked.
	ErrLinkRevoked = errors.New("cairn: link revoked")

	// ErrInvalidCode reports that a code failed alphabet or length validation
	// before any store lookup was attempted (SR-21).
	ErrInvalidCode = errors.New("cairn: invalid code")

	// ErrCodeSpaceExhausted reports that Create retried past its configured
	// ceiling without finding a free code (SR-19).
	ErrCodeSpaceExhausted = errors.New("cairn: code space exhausted after max attempts")

	// ErrStoreUnavailable reports that the store could not service a request.
	// cairn fails closed on it: never a redirect, never a success (SR-20).
	ErrStoreUnavailable = errors.New("cairn: store unavailable")

	// ErrDestinationRejected reports that a destination was rejected, either
	// by ParseDestination's own syntax checks or by a Policy. Use
	// [RejectReasonFrom] to recover the specific [RejectReason] without
	// matching on the error string.
	ErrDestinationRejected = errors.New("cairn: destination rejected by policy")

	// ErrVanityReserved reports that a requested vanity code is on the
	// reserved list.
	ErrVanityReserved = errors.New("cairn: vanity code is reserved")

	// ErrVanityLength reports that a requested vanity code's length collides
	// with the configured generated length, which would let a vanity code
	// shadow — or be shadowed by — a randomly generated one (ADR-0011).
	ErrVanityLength = errors.New("cairn: vanity code length collides with the generated length")
)

// RejectReason classifies why a destination was rejected. Values are a stable,
// exported string enum: a host may serialize one directly into an API
// response without cairn ever changing its wording underneath them (IR-04).
type RejectReason string

// RejectReason values. Each is cited by the requirement it enforces; four are
// applied by ParseDestination's syntax checks (SR-05, SR-06, SR-08, SR-10) and
// the rest by a Policy (SR-07, SR-09), which needs a network and a context
// that ParseDestination deliberately does not have.
const (
	// ReasonScheme reports a scheme outside the allowlist (SR-05).
	ReasonScheme RejectReason = "scheme_not_allowed"

	// ReasonUserinfo reports userinfo embedded in the URL (SR-06).
	ReasonUserinfo RejectReason = "credentials_in_url"

	// ReasonPrivateAddress reports a host that resolves to a private,
	// loopback, link-local, or otherwise internal address (SR-07).
	ReasonPrivateAddress RejectReason = "private_or_internal_host"

	// ReasonTooLong reports a destination longer than the configured maximum
	// (SR-08).
	ReasonTooLong RejectReason = "destination_too_long"

	// ReasonOwnDomain reports a destination on the service's own configured
	// domain (SR-09).
	ReasonOwnDomain RejectReason = "own_domain"

	// ReasonControlChars reports a control character or whitespace anywhere in
	// the destination, including a percent-decoded form in the host (SR-10).
	ReasonControlChars RejectReason = "control_characters"

	// ReasonResolveFailed reports that name resolution failed or could not be
	// attempted. A Policy fails closed on this reason rather than allowing
	// through an address it could not check (SR-20).
	ReasonResolveFailed RejectReason = "host_resolution_failed"

	// ReasonNotAllowlisted reports a destination outside a configured
	// allowlist. Unlike the other reasons, this one drives an Interstitial
	// decision rather than a Deny (FR-13).
	ReasonNotAllowlisted RejectReason = "outside_allowlist"
)

// RejectionError wraps [ErrDestinationRejected] with the specific
// [RejectReason] that caused it, so a host can render a targeted message
// while errors.Is(err, ErrDestinationRejected) still holds.
type RejectionError struct {
	Reason RejectReason
}

// NewRejectionError returns an error reporting that a destination was
// rejected for reason. errors.Is(err, ErrDestinationRejected) is true for the
// result, and [RejectReasonFrom] recovers reason from it.
func NewRejectionError(reason RejectReason) error {
	return &RejectionError{Reason: reason}
}

// Error implements the error interface.
func (e *RejectionError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDestinationRejected, e.Reason)
}

// Unwrap makes errors.Is(err, ErrDestinationRejected) true for any error
// produced by [NewRejectionError], including one wrapped further by a caller.
func (e *RejectionError) Unwrap() error {
	return ErrDestinationRejected
}

// RejectReasonFrom reports the [RejectReason] carried by err, if any. It
// unwraps err with errors.As, so a reason survives arbitrary additional
// wrapping via fmt.Errorf("%w", ...).
func RejectReasonFrom(err error) (RejectReason, bool) {
	var re *RejectionError
	if errors.As(err, &re) {
		return re.Reason, true
	}
	return "", false
}
