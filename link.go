package cairn

import "time"

// Link is the stored record. Its destination is immutable: repointing a code
// is not an operation this library offers (T-03).
type Link struct {
	// Code is the short code that resolves to Dest.
	Code Code

	// Dest is the destination. It redacts itself in any formatting, logging
	// or marshalling of a Link (SR-15).
	Dest Destination

	// OwnerID identifies who created the link. It is data: cairn compares and
	// stores it, and attaches no meaning to it. In particular, [Shortener]
	// never uses OwnerID to authorize an operation — Revoke does not check
	// ownership, and neither does anything else here (ADR-0010). A host that
	// needs authorization enforces it itself, before calling into cairn.
	OwnerID string

	// Vanity reports whether Code was chosen by the caller rather than
	// generated (ADR-0011).
	Vanity bool

	// Interstitial reports whether Policy.Evaluate classified Dest as
	// Interstitial at creation. The classification is fixed here rather than
	// re-evaluated on resolve, which keeps DNS off the redirect path and
	// keeps a link's behaviour stable for its lifetime — a destination added
	// to an allowlist later does not retroactively stop warning; recreating
	// the link does (ADR-0014). cairnhttp reads this field to decide whether
	// to render a warning page; Resolve itself does not act on it.
	Interstitial bool

	// CreatedAt is when the link was created.
	CreatedAt time.Time

	// ExpiresAt is the link's logical expiry, evaluated against the
	// configured clock — never the store's, which is not cairn's to trust.
	// The zero value means the link never expires. A store's own TTL, where
	// it has one, is set past ExpiresAt as a grace period; ExpiresAt alone is
	// authoritative for whether the link is active (ADR-0009).
	ExpiresAt time.Time

	// RevokedAt is when the link was explicitly revoked. The zero value means
	// it has not been.
	RevokedAt time.Time
}

// IsExpired reports whether the link's logical expiry has passed as of now.
// A zero ExpiresAt means the link never expires. The boundary instant itself
// — now equal to ExpiresAt — counts as expired.
func (l *Link) IsExpired(now time.Time) bool {
	if l.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(l.ExpiresAt)
}

// IsRevoked reports whether the link has been explicitly revoked.
func (l *Link) IsRevoked() bool {
	return !l.RevokedAt.IsZero()
}

// IsActive reports whether the link may still be resolved as of now: neither
// expired nor revoked.
func (l *Link) IsActive(now time.Time) bool {
	return !l.IsExpired(now) && !l.IsRevoked()
}
