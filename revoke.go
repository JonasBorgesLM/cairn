package cairn

import "context"

// Revoke marks code's link revoked. The record is kept, including its
// destination, so the next Resolve fails with ErrLinkRevoked — with nothing
// to invalidate anywhere, because there is no read cache in front of the
// store to begin with (SR-23) — while the record stays available for the
// grace window, for a support conversation or an audit trail (ADR-0009).
//
// Revoke does not check ownership. It takes a code and revokes it: the caller
// must already have authorized this operation, and cairn does not verify that
// the caller owns the link (ADR-0010). A host exposing revocation to end
// users enforces that check itself, before calling this.
func (s *Shortener) Revoke(ctx context.Context, code Code) error {
	return s.revoke(ctx, code, false)
}

// RevokeAndPurge revokes code's link exactly as Revoke does, and additionally
// clears the stored destination in the same write. Use this when the reason
// to revoke is that the destination itself is toxic — it carried a secret,
// say — rather than merely wrong; Revoke's default keeps the destination
// because that is the more common reason to revoke (ADR-0009).
//
// After this call, the record still exists and Resolve still reports
// ErrLinkRevoked, but the destination is gone: Link.Dest.IsZero() is true for
// any subsequent Load of this code.
//
// The same ownership caveat as Revoke applies: cairn does not check it.
func (s *Shortener) RevokeAndPurge(ctx context.Context, code Code) error {
	return s.revoke(ctx, code, true)
}

func (s *Shortener) revoke(ctx context.Context, code Code, purge bool) error {
	if err := s.alphabet.Validate(code); err != nil {
		return err
	}
	return s.store.Revoke(ctx, code, s.clock(), purge)
}
