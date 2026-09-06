package cairn

import (
	"context"
	"fmt"
	"time"
)

// Shortener creates and resolves short links against a Store. Construct one
// with New; there are no exported setters, so a Shortener's configuration is
// fixed for its lifetime once New returns (NFR-04, NFR-08).
type Shortener struct {
	store     Store
	destIndex DestIndex // set only when dedup is enabled

	policy    Policy
	generator CodeGenerator
	alphabet  Alphabet

	codeLength      int
	expectedLinks   int64
	maxSaveAttempts int
	defaultTTL      time.Duration
	expiryGrace     time.Duration

	vanityEnabled  bool
	vanityMinLen   int
	vanityMaxLen   int
	vanityReserved map[string]bool

	dedup   bool
	counter Counter
	hooks   Hooks
	clock   func() time.Time
}

// options accumulates Option values before New validates and freezes them
// into a Shortener.
type options struct {
	policy          Policy
	generator       CodeGenerator
	alphabet        Alphabet
	alphabetSet     bool
	codeLength      int
	codeLengthSet   bool
	expectedLinks   int64
	maxSaveAttempts int
	defaultTTL      time.Duration
	expiryGrace     time.Duration
	vanityEnabled   bool
	vanityMinLen    int
	vanityMaxLen    int
	vanityReserved  []string
	dedup           bool
	counter         Counter
	hooks           Hooks
	clock           func() time.Time
}

// Option configures a Shortener at construction. See New.
type Option func(*options)

// WithPolicy sets the destination Policy. Required: New fails without one
// (ADR-0005).
func WithPolicy(p Policy) Option { return func(o *options) { o.policy = p } }

// WithGenerator sets a custom CodeGenerator, overriding the default
// (rejection-sampling over crypto/rand). Because cairn cannot introspect an
// opaque generator's alphabet or length, supplying one skips the keyspace
// density check of ADR-0002 — the caller is responsible for that analysis.
func WithGenerator(g CodeGenerator) Option { return func(o *options) { o.generator = g } }

// WithAlphabet sets the Alphabet the default generator draws from. Default
// AlphabetBase62. Selecting AlphabetUnambiguous without an explicit
// WithCodeLength raises the default code length to 11 (ADR-0004).
func WithAlphabet(a Alphabet) Option {
	return func(o *options) { o.alphabet = a; o.alphabetSet = true }
}

// WithCodeLength sets the generated code length explicitly, overriding the
// alphabet's own default length. Still subject to the density check of
// ADR-0002 against WithExpectedLinks.
func WithCodeLength(n int) Option {
	return func(o *options) { o.codeLength = n; o.codeLengthSet = true }
}

// WithExpectedLinks feeds the keyspace density check (SR-02, ADR-0002). Zero
// (the default) skips the check.
func WithExpectedLinks(n int64) Option { return func(o *options) { o.expectedLinks = n } }

// WithMaxSaveAttempts bounds the retry loop on a generated-code collision.
// Default 5 (SR-19).
func WithMaxSaveAttempts(n int) Option { return func(o *options) { o.maxSaveAttempts = n } }

// WithDefaultTTL sets the TTL applied to a Create call that does not specify
// its own via WithTTL or WithExpiresAt. Default zero, meaning links never
// expire unless a create call says otherwise.
func WithDefaultTTL(d time.Duration) Option { return func(o *options) { o.defaultTTL = d } }

// WithExpiryGrace sets the grace window a store's native TTL is set past a
// link's logical ExpiresAt, so an expired-but-not-yet-collected link still
// resolves to ErrLinkExpired rather than ErrCodeNotFound. Default 30 days
// (ADR-0009).
func WithExpiryGrace(d time.Duration) Option { return func(o *options) { o.expiryGrace = d } }

// DefaultVanityReserved is always rejected with ErrVanityReserved, whether or
// not a caller passes its own list to WithVanity — merged in, never
// something a caller has to remember to ask for. Without it, a vanity code
// shadows one of the host's own routes, which is a routing bug that presents
// as a security incident (ADR-0011).
var DefaultVanityReserved = []string{
	"admin", "api", "health", "login", "static", "assets",
	"robots.txt", "favicon.ico", ".well-known",
}

// WithVanity enables caller-chosen codes in [minLen, maxLen]. A vanity code
// is a public name a caller picks for memorability, never a capability: it
// is exactly as guessable as the string chosen, and a link that must be
// unguessable must not be vanity (ADR-0011).
//
// reserved is merged with DefaultVanityReserved and excluded outright
// (ErrVanityReserved); pass nil for just the default list. The range must
// not include the generated code length (SR-04, ADR-0011).
func WithVanity(minLen, maxLen int, reserved []string) Option {
	return func(o *options) {
		o.vanityEnabled = true
		o.vanityMinLen = minLen
		o.vanityMaxLen = maxLen
		o.vanityReserved = reserved
	}
}

// WithDeduplication enables returning an existing code for a destination
// already shortened by the same owner, scoped per owner (ADR-0013). Requires
// a Store that implements DestIndex; New fails if it does not. Default false
// — deduplication is an existence oracle (SR-17).
func WithDeduplication(enabled bool) Option { return func(o *options) { o.dedup = enabled } }

// WithCounter sets the Counter invoked by Shortener.Count. See Count's
// documentation for how a host is expected to call it (FR-14, ADR-0012).
func WithCounter(c Counter) Option { return func(o *options) { o.counter = c } }

// WithHooks sets the lifecycle event callbacks. Default zero value, in which
// every hook is a no-op.
func WithHooks(h Hooks) Option { return func(o *options) { o.hooks = h } }

// WithClock overrides the clock used to evaluate expiry and to timestamp
// created and revoked records. For tests; production code has no reason to
// call it, and cairn holds no global clock (NFR-04).
func WithClock(now func() time.Time) Option { return func(o *options) { o.clock = now } }

// defaultMaxSaveAttempts is SR-19's bound on the generated-code retry loop.
const defaultMaxSaveAttempts = 5

// defaultExpiryGrace is ADR-0009's grace window past a link's logical expiry.
const defaultExpiryGrace = 30 * 24 * time.Hour

// New validates the configuration eagerly and returns a ready Shortener. It
// fails if store is nil, if no Policy is set (ADR-0005), if the alphabet and
// code length give a keyspace that is unsafe for the declared expected link
// count (ADR-0002), if deduplication is enabled against a store that is not a
// DestIndex (ADR-0013), if the vanity length range overlaps the generated
// length (SR-04), or if store implements EvictionChecker and reports an
// evicting policy (ADR-0008). There is no option to construct a Shortener
// that defers any of these checks to first use (NFR-08).
func New(store Store, opts ...Option) (*Shortener, error) {
	if store == nil {
		return nil, fmt.Errorf("cairn: New requires a non-nil Store")
	}

	o := options{
		maxSaveAttempts: defaultMaxSaveAttempts,
		expiryGrace:     defaultExpiryGrace,
		clock:           time.Now,
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.policy == nil {
		return nil, fmt.Errorf("cairn: New requires a Policy (ADR-0005); a Shortener with no destination policy is what this library exists not to be")
	}

	alphabet := AlphabetBase62
	if o.alphabetSet {
		alphabet = o.alphabet
	}
	codeLength := alphabet.DefaultLength()
	if o.codeLengthSet {
		codeLength = o.codeLength
	}

	generator := o.generator
	if generator == nil {
		if err := CheckKeyspaceDensity(alphabet, codeLength, o.expectedLinks); err != nil {
			return nil, err
		}
		g, err := NewRandomGenerator(alphabet, codeLength)
		if err != nil {
			return nil, err
		}
		generator = g
	}

	if o.vanityEnabled {
		if o.vanityMinLen > o.vanityMaxLen {
			return nil, fmt.Errorf("cairn: vanity length range [%d, %d] is invalid: min exceeds max", o.vanityMinLen, o.vanityMaxLen)
		}
		if codeLength >= o.vanityMinLen && codeLength <= o.vanityMaxLen {
			return nil, fmt.Errorf(
				"cairn: vanity length range [%d, %d] overlaps the generated code length %d (SR-04); "+
					"a vanity code of the generated length could collide with, or be squatted ahead of, a generated one",
				o.vanityMinLen, o.vanityMaxLen, codeLength)
		}
	}

	var destIndex DestIndex
	if o.dedup {
		di, ok := store.(DestIndex)
		if !ok {
			return nil, fmt.Errorf("cairn: WithDeduplication requires a Store implementing DestIndex (ADR-0013), but %T does not", store)
		}
		destIndex = di
	}

	if ec, ok := store.(EvictionChecker); ok {
		if err := ec.EvictionCheck(context.Background()); err != nil {
			return nil, fmt.Errorf("cairn: store eviction check failed (ADR-0008): %w", err)
		}
	}

	if o.maxSaveAttempts <= 0 {
		return nil, fmt.Errorf("cairn: WithMaxSaveAttempts must be positive, got %d", o.maxSaveAttempts)
	}

	reserved := make(map[string]bool, len(DefaultVanityReserved)+len(o.vanityReserved))
	for _, r := range DefaultVanityReserved {
		reserved[r] = true
	}
	for _, r := range o.vanityReserved {
		reserved[r] = true
	}

	return &Shortener{
		store:           store,
		destIndex:       destIndex,
		policy:          o.policy,
		generator:       generator,
		alphabet:        alphabet,
		codeLength:      codeLength,
		expectedLinks:   o.expectedLinks,
		maxSaveAttempts: o.maxSaveAttempts,
		defaultTTL:      o.defaultTTL,
		expiryGrace:     o.expiryGrace,
		vanityEnabled:   o.vanityEnabled,
		vanityMinLen:    o.vanityMinLen,
		vanityMaxLen:    o.vanityMaxLen,
		vanityReserved:  reserved,
		dedup:           o.dedup,
		counter:         o.counter,
		hooks:           o.hooks,
		clock:           o.clock,
	}, nil
}

// Count invokes the configured Counter, if any, recording a resolution of c.
// It runs the count on a context derived from ctx with context.WithoutCancel,
// so a request context cancelled by the completed response it counted does
// not race the count itself (ADR-0012). Resolve never calls this; cairnhttp
// calls it off the resolve critical path, after the redirect response has
// been written (FR-14).
func (s *Shortener) Count(ctx context.Context, c Code) error {
	if s.counter == nil {
		return nil
	}
	return s.counter.Count(context.WithoutCancel(ctx), c)
}
