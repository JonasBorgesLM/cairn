package cairn

import "context"

// Resolve looks up code and returns its Link, or one of three distinct
// failures: ErrInvalidCode, ErrCodeNotFound, ErrLinkRevoked or ErrLinkExpired
// (FR-02, FR-09).
//
// Alphabet and length validation happens before code ever reaches the store
// (SR-21) — reversing that order would still work for a well-formed code, but
// would let a malformed one reach the store on every attempt, and this
// ordering is what makes a scanner's malformed probes free instead.
func (s *Shortener) Resolve(ctx context.Context, code Code) (*Link, error) {
	if err := s.alphabet.Validate(code); err != nil {
		return nil, err
	}

	link, err := s.store.Load(ctx, code)
	if err != nil {
		return nil, err
	}

	if link.IsRevoked() {
		return nil, ErrLinkRevoked
	}
	if link.IsExpired(s.clock()) {
		return nil, ErrLinkExpired
	}

	s.fireResolve(ctx, code, link.Dest)
	return link, nil
}

func (s *Shortener) fireResolve(ctx context.Context, c Code, d Destination) {
	if s.hooks.OnResolve != nil {
		s.hooks.OnResolve(ctx, ResolveEvent{Code: c, Dest: d})
	}
}
