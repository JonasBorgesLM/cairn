package cairn

import (
	"context"
	"time"
)

// Cursor is an opaque pagination token returned by OwnerLister.ListByOwner. A
// consumer passes it back unmodified to continue a listing; its contents are
// a store implementation's own concern.
type Cursor string

// Store persists links. Implementations must be safe for concurrent use.
//
// Save must be conditional: it creates the record only if the code is absent,
// and returns ErrCodeExists otherwise. An implementation that overwrites an
// existing code is a defect, not a variation — an unconditional write
// silently repoints a distributed link at an attacker's destination (T-03),
// the worst outcome this library can have (SR-18).
//
// Revoke sets RevokedAt on the record and keeps it, so a revoked link stays
// distinguishable from one that never existed for as long as the record
// lives (ADR-0009). When purgeDestination is true, it additionally clears the
// stored destination in the same write — for the case where the reason to
// revoke is that the URL itself is toxic, not merely wrong.
//
// Every method takes ctx first and must fail closed: an unreachable store
// returns an error, never a zero value dressed up as success (SR-20).
type Store interface {
	Save(ctx context.Context, l *Link) error
	Load(ctx context.Context, c Code) (*Link, error)
	Revoke(ctx context.Context, c Code, at time.Time, purgeDestination bool) error
}

// OwnerLister backs listing by owner (FR-10). It is optional: a Shortener
// type-asserts a Store against it only when a caller actually asks to list by
// owner's capability at construction, so a missing capability is a startup
// error rather than a runtime surprise.
//
// ListByOwner is a filter, not authorization: an ownerID is taken as given,
// and the caller must already have established that the ownerID it passes is
// one the current caller may see. Exposing this to an unauthenticated caller
// with an attacker-chosen ownerID is a vulnerability in the host, not in this
// filter (ADR-0010).
type OwnerLister interface {
	ListByOwner(ctx context.Context, ownerID string, after Cursor, limit int) ([]*Link, Cursor, error)
}

// DestIndex backs deduplication (FR-15, SR-17). Required only when
// deduplication is enabled; WithDeduplication fails at New if the store does
// not implement it.
//
// ownerID scopes both methods (ADR-0013): deduplication is per owner, not
// global, so that two different owners shortening the same destination get
// two different codes and only a single owner submitting the same URL twice
// gets one. An empty ownerID scopes to the unowned bucket, matching
// Link.OwnerID's own "empty means unowned" convention. A store implementation
// keys on some function of (ownerID, the normalized destination) — never on
// the destination alone, and never on the raw URL text, since a key name is
// outside SR-15's redaction surface (it shows up in SCAN output, MONITOR, and
// slow-query logs).
type DestIndex interface {
	LookupByDest(ctx context.Context, ownerID string, d Destination) (Code, error)
	IndexDest(ctx context.Context, ownerID string, d Destination, c Code) error
}

// EvictionChecker verifies the backend will not silently discard links under
// memory pressure. Shortener.New calls it, where a Store implements it,
// refusing to start against a misconfigured backend (mirrors moat's
// rate-limit store check, T-12, ADR-0008).
type EvictionChecker interface {
	EvictionCheck(ctx context.Context) error
}

// Counter records a resolution. It is deliberately not part of Store: it is
// best-effort, invoked off the critical path, and its failures never affect a
// redirect (FR-14, ADR-0012).
type Counter interface {
	Count(ctx context.Context, c Code) error
}
