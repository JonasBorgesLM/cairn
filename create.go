package cairn

import (
	"context"
	"errors"
	"time"
)

// createOptions accumulates CreateOption values for one Create call.
type createOptions struct {
	ownerID    string
	vanityCode Code
	ttl        time.Duration
	ttlSet     bool
	expiresAt  time.Time
}

// CreateOption configures a single Create call.
type CreateOption func(*createOptions)

// WithOwner attaches an owner identifier to the created link (ADR-0010).
func WithOwner(id string) CreateOption { return func(o *createOptions) { o.ownerID = id } }

// WithVanityCode requests a specific, caller-chosen code rather than a
// generated one. Requires WithVanity to have been configured at New.
func WithVanityCode(c Code) CreateOption { return func(o *createOptions) { o.vanityCode = c } }

// WithTTL sets this link's time-to-live, overriding the Shortener's
// WithDefaultTTL for this call only.
func WithTTL(d time.Duration) CreateOption {
	return func(o *createOptions) { o.ttl = d; o.ttlSet = true }
}

// WithExpiresAt sets this link's absolute expiry, overriding both
// WithDefaultTTL and any WithTTL passed to the same call.
func WithExpiresAt(t time.Time) CreateOption { return func(o *createOptions) { o.expiresAt = t } }

// Create shortens rawURL into a Link. See docs/ARCHITECTURE.md §5.1 for the
// full flow; the two orderings load-bearing enough to have their own tests
// are: Store.Save happens before dedup indexing (a dangling index entry is
// recoverable, one pointing at an unsaved link is not), and only a generated
// code is retried on collision — retrying a vanity code would silently issue
// a different code than the caller asked for.
func (s *Shortener) Create(ctx context.Context, rawURL string, opts ...CreateOption) (*Link, error) {
	var co createOptions
	for _, opt := range opts {
		opt(&co)
	}

	// Steps 1-2: length, scheme, userinfo and control-character checks
	// (SR-05, SR-06, SR-08, SR-10).
	dest, err := ParseDestination(rawURL)
	if err != nil {
		return nil, err
	}

	// Step 3: normalize (FR-16, ADR-0015), then re-validate the normalized
	// string into a fresh Destination. This second parse is expected to
	// always succeed — normalization only narrows what was already valid —
	// and is defensive rather than load-bearing.
	if u, uerr := dest.URL(); uerr == nil {
		if renorm, rerr := ParseDestination(NormalizeURL(u).String()); rerr == nil {
			dest = renorm
		}
	}

	// Step 4: Policy.Evaluate (SR-07, SR-09).
	decision, reason, err := s.policy.Evaluate(ctx, dest)
	if err != nil {
		return nil, err
	}
	var interstitial bool
	switch decision {
	case Deny:
		s.fireReject(ctx, dest, reason)
		return nil, NewRejectionError(reason)
	case Interstitial:
		interstitial = true
	case Allow:
	}

	// Step 5: dedup lookup, scoped per owner (SR-17, ADR-0013).
	if s.dedup {
		if code, lerr := s.destIndex.LookupByDest(ctx, co.ownerID, dest); lerr == nil {
			if link, lerr2 := s.store.Load(ctx, code); lerr2 == nil {
				return link, nil
			}
			// The index pointed at a link that is no longer loadable.
			// Harmless (ADR-0013): fall through and create a new one.
		} else if !errors.Is(lerr, ErrCodeNotFound) {
			return nil, lerr
		}
	}

	now := s.clock()
	expiresAt := s.resolveExpiry(now, co)

	var link *Link
	if co.vanityCode != "" {
		link, err = s.createVanity(ctx, dest, co, now, expiresAt, interstitial)
	} else {
		link, err = s.createGenerated(ctx, dest, co, now, expiresAt, interstitial)
	}
	if err != nil {
		return nil, err
	}

	// Step 8: dedup indexing, after Save, best-effort (ADR-0013). A failure
	// here leaves a dangling index entry: the next lookup misses and a new
	// link is created, which is harmless, so it is not surfaced as a Create
	// error -- the link itself was already saved successfully.
	if s.dedup {
		// #nosec G104 -- deliberate: ADR-0013's best-effort index write; a failure is harmless, not a Create error.
		//nolint:errcheck // see the #nosec comment above; both suppressions are needed, gosec runs standalone in CI as well as inside golangci-lint.
		s.destIndex.IndexDest(ctx, co.ownerID, dest, link.Code)
	}

	s.fireCreate(ctx, link.Code, dest, co.ownerID)
	return link, nil
}

func (s *Shortener) resolveExpiry(now time.Time, co createOptions) time.Time {
	if !co.expiresAt.IsZero() {
		return co.expiresAt
	}
	ttl := s.defaultTTL
	if co.ttlSet {
		ttl = co.ttl
	}
	if ttl > 0 {
		return now.Add(ttl)
	}
	return time.Time{}
}

// createVanity validates and saves a caller-chosen code. It never retries: a
// collision is returned to the caller as ErrCodeExists directly (ADR-0011).
func (s *Shortener) createVanity(ctx context.Context, dest Destination, co createOptions, now, expiresAt time.Time, interstitial bool) (*Link, error) {
	code := co.vanityCode
	if err := s.alphabet.Validate(code); err != nil {
		return nil, err
	}
	if len(code) == s.codeLength {
		return nil, ErrVanityLength
	}
	if s.vanityReserved[string(code)] {
		return nil, ErrVanityReserved
	}

	link := &Link{
		Code:         code,
		Dest:         dest,
		OwnerID:      co.ownerID,
		Vanity:       true,
		Interstitial: interstitial,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
	}
	if err := s.store.Save(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// createGenerated draws a code from the configured CodeGenerator and saves
// it, retrying on a collision up to maxSaveAttempts (SR-18, SR-19). Any
// non-collision store error returns immediately, with no retry and no link.
func (s *Shortener) createGenerated(ctx context.Context, dest Destination, co createOptions, now, expiresAt time.Time, interstitial bool) (*Link, error) {
	for attempt := 1; attempt <= s.maxSaveAttempts; attempt++ {
		code, err := s.generator.Generate(ctx)
		if err != nil {
			return nil, err
		}
		link := &Link{
			Code:         code,
			Dest:         dest,
			OwnerID:      co.ownerID,
			Vanity:       false,
			Interstitial: interstitial,
			CreatedAt:    now,
			ExpiresAt:    expiresAt,
		}

		err = s.store.Save(ctx, link)
		if err == nil {
			return link, nil
		}
		if !errors.Is(err, ErrCodeExists) {
			return nil, err
		}

		s.fireRetry(ctx, code, attempt)
		if attempt == s.maxSaveAttempts {
			return nil, ErrCodeSpaceExhausted
		}
	}
	panic("cairn: unreachable: createGenerated loop exited without returning")
}

func (s *Shortener) fireCreate(ctx context.Context, c Code, d Destination, ownerID string) {
	if s.hooks.OnCreate != nil {
		s.hooks.OnCreate(ctx, CreateEvent{Code: c, Dest: d, OwnerID: ownerID})
	}
}

func (s *Shortener) fireReject(ctx context.Context, d Destination, reason RejectReason) {
	if s.hooks.OnReject != nil {
		s.hooks.OnReject(ctx, RejectEvent{Dest: d, Reason: reason})
	}
}

func (s *Shortener) fireRetry(ctx context.Context, c Code, attempt int) {
	if s.hooks.OnRetry != nil {
		s.hooks.OnRetry(ctx, RetryEvent{Code: c, Attempt: attempt})
	}
}
