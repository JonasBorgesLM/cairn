# cairn — Architecture

**Version:** 0.1
**Status:** proposed — signatures below are a design artifact, not committed API.
**Companions:** [`../REQUIREMENTS.md`](../REQUIREMENTS.md) ·
[`THREAT-MODEL.md`](THREAT-MODEL.md) · [`adr/`](adr/README.md)

Every signature here is a proposal to be reviewed before it is written. Bodies
are deliberately absent: this phase decides shape, not implementation.

---

## 1. Module and package layout

```
cairn/                        module github.com/JonasBorgesLM/cairn   (go 1.24)
│                             deps: github.com/JonasBorgesLM/moat  (first-party, ADR-0007)
├── cairn.go                  Shortener, Options, sentinel errors
├── link.go                   Link, Code
├── alphabet.go               Alphabet
├── destination.go            Destination — redacts by default
├── code.go                   CodeGenerator, random generator
├── policy.go                 Policy interface + Decision  (the interface lives here — §2)
├── store.go                  Store + optional capability interfaces
├── hooks.go                  Hooks, event types
├── policy/                   policy implementations (imports cairn)
├── memstore/                 in-memory Store for consumer tests
└── cairnhttp/                optional redirect handler (imports net/http)

cairn/redisstore/             module github.com/JonasBorgesLM/cairn/redisstore
                              deps: go-redis, testcontainers (test only)
```

Two modules, not one, for the same reason as `moat` and `crier`: a consumer who
brings their own store should not inherit `go-redis` in their dependency graph
(NFR-01, NFR-03). Independently tagged; a green build in one says nothing about
the other.

### Why `Policy` is declared in the core but implemented in `policy/`

The obvious layout — `Policy` and `Decision` in `policy/`, `cairn` importing it —
is an import cycle, because policy evaluates a `cairn.Destination`. The Go idiom
resolves it: **the consumer declares the interface**. `cairn` declares `Policy`
and `Decision`; `policy` imports `cairn` and provides implementations. The
dependency runs one way, from concrete to abstract, and a consumer can supply
their own policy without importing `policy/` at all.

This has one visible consequence, and it is deliberate: `cairn.New` **cannot
default to `policy.Default()`**, because that would be the cycle again. So it
does not default at all — constructing a `Shortener` without a policy is a
configuration error and `New` returns it. A shortener with no destination policy
is precisely the thing this library exists not to be, and the layout makes the
unsafe configuration unrepresentable rather than merely discouraged. The cost is
one extra line in every consumer's setup, and it is worth it (ADR-0005).

---

## 2. Core types

### 2.1 Code and Alphabet

```go
// Code is a short code. It is untrusted input on the resolve path until
// Alphabet.Validate has accepted it (SR-21).
type Code string

func (c Code) String() string

// Alphabet is a validated set of runes usable in a code. Constructing one
// rejects duplicates, non-ASCII runes, and anything outside [A-Za-z0-9_-].
type Alphabet struct{ /* unexported */ }

func NewAlphabet(runes string) (Alphabet, error)

// AlphabetBase62 is the default: [0-9A-Za-z], 5.954 bits per rune.
var AlphabetBase62 Alphabet

// AlphabetUnambiguous omits 0 O o I l 1 for transcription contexts,
// at 5.807 bits per rune. See ADR-0004 for the cost and when it is worth it.
var AlphabetUnambiguous Alphabet

func (a Alphabet) Size() int
func (a Alphabet) BitsPerRune() float64
func (a Alphabet) Contains(r rune) bool

// Validate reports whether every rune of c is in the alphabet. It is called
// before a code is concatenated into a store key, never after (SR-21).
func (a Alphabet) Validate(c Code) error
```

### 2.2 Destination

Built on `moat/secret.Value` (ADR-0007, IR-03). The raw URL is held masked; the
fields that are safe to log — scheme and host — are held in the clear so that a
redacted rendering is still useful in an incident.

```go
// Destination is a validated http/https URL that redacts itself through every
// formatting, logging and marshalling path the standard library offers (SR-15).
//
// The raw form comes out only through URL or Raw, which are explicit and
// visible in review.
type Destination struct{ /* unexported: secret.Value + scheme + host */ }

// ParseDestination checks syntax and shape only — scheme allowlist, userinfo,
// control characters, length (SR-05, SR-06, SR-08, SR-10). It performs no name
// resolution and applies no Policy; that is Policy's job, and it needs a
// context.
func ParseDestination(raw string) (Destination, error)

func (d Destination) URL() (*url.URL, error) // explicit unmask
func (d Destination) Raw() string            // explicit unmask
func (d Destination) Scheme() string         // safe to log
func (d Destination) Host() string           // safe to log
func (d Destination) IsZero() bool
func (d Destination) Equal(other Destination) bool

// Redacting surface — every one of these writes the redacted form.
func (d Destination) String() string
func (d Destination) GoString() string
func (d Destination) Format(f fmt.State, verb rune)
func (d Destination) LogValue() slog.Value
func (d Destination) MarshalText() ([]byte, error)
func (d Destination) MarshalJSON() ([]byte, error)
```

The redacted rendering is `https://example.com/[REDACTED]` — scheme and host
kept, everything after the authority replaced. Path and query are where the
tokens live; host is what an operator needs to triage. `MarshalJSON` and
`MarshalText` emit the same string, so a `Link` serialized into a log or an
audit record cannot leak by a route that `String` does not cover.

`UnmarshalText`/`UnmarshalJSON` refuse to load a value containing the redaction
placeholder, mirroring `secret.ErrRedacted`: a round trip through JSON must not
silently turn a redacted rendering back into a "destination".

### 2.3 Link

```go
// Link is the stored record. Its destination is immutable: repointing a code is
// not an operation this library offers (T-03).
type Link struct {
    Code      Code
    Dest      Destination
    OwnerID   string      // first-class, ADR-0010
    Vanity    bool
    CreatedAt time.Time
    ExpiresAt time.Time   // zero means never
    RevokedAt time.Time   // zero means not revoked
}

func (l *Link) IsExpired(now time.Time) bool
func (l *Link) IsRevoked() bool
func (l *Link) IsActive(now time.Time) bool
```

`ExpiresAt` is a *logical* expiry carried in the record. The store's own TTL is
set to `ExpiresAt + grace`, so an expired link is still readable for the grace
window and resolves as `ErrLinkExpired` rather than `ErrCodeNotFound` (ADR-0009).
After the grace window the record is collected and the two become
indistinguishable — which is the correct end state for privacy, reached
deliberately rather than immediately.

### 2.4 Errors

All comparable with `errors.Is` (NFR-07).

```go
var (
    ErrCodeExists         = errors.New("cairn: code already exists")
    ErrCodeNotFound       = errors.New("cairn: code not found")
    ErrLinkExpired        = errors.New("cairn: link expired")
    ErrLinkRevoked        = errors.New("cairn: link revoked")
    ErrInvalidCode        = errors.New("cairn: invalid code")
    ErrCodeSpaceExhausted = errors.New("cairn: code space exhausted after max attempts")
    ErrStoreUnavailable   = errors.New("cairn: store unavailable")
    ErrDestinationRejected= errors.New("cairn: destination rejected by policy")
    ErrVanityReserved     = errors.New("cairn: vanity code is reserved")
    ErrVanityLength       = errors.New("cairn: vanity code length collides with the generated length")
)
```

`ErrDestinationRejected` wraps a `RejectReason` so a host can render a specific
message without matching on strings, while `errors.Is(err, ErrDestinationRejected)`
stays true.

---

## 3. Interfaces

### 3.1 Store — the required contract

```go
// Store persists links. Implementations must be safe for concurrent use.
//
// Save must be conditional: it creates the record only if the code is absent,
// and returns ErrCodeExists otherwise. An implementation that overwrites is a
// defect, not a variation (SR-18).
type Store interface {
    Save(ctx context.Context, l *Link) error
    Load(ctx context.Context, c Code) (*Link, error)
    Revoke(ctx context.Context, c Code, at time.Time) error
}
```

Three methods, and no more, because every method here is a method every store
implementer must write correctly. Optional capabilities are separate interfaces
that a store may implement and the `Shortener` type-asserts at construction —
never per call.

### 3.2 Optional capability interfaces

```go
// OwnerLister backs listing by owner (FR-10). Required only when the host asks
// for it; asserted at construction, so a missing capability is a startup error
// rather than a runtime surprise.
type OwnerLister interface {
    ListByOwner(ctx context.Context, ownerID string, after Cursor, limit int) ([]*Link, Cursor, error)
}

// DestIndex backs deduplication (FR-15, SR-17). Required when dedup is enabled;
// WithDeduplication fails at New if the store does not implement it.
type DestIndex interface {
    LookupByDest(ctx context.Context, d Destination) (Code, error)
    IndexDest(ctx context.Context, d Destination, c Code) error
}

// EvictionChecker verifies the backend will not silently discard links under
// memory pressure. Mirrors moat's rate-limit store check (T-12, ADR-0008).
type EvictionChecker interface {
    EvictionCheck(ctx context.Context) error
}

// Counter records a resolution. It is deliberately not part of Store: it is
// best-effort, it is called off the critical path, and its failures never
// affect a redirect (FR-14, ADR-0012).
type Counter interface {
    Count(ctx context.Context, c Code) error
}
```

### 3.3 CodeGenerator

```go
// CodeGenerator produces codes.
//
// An implementation MUST draw from a cryptographically secure source. A
// counter, a timestamp, a hash of the destination, or a math/rand generator all
// reintroduce T-02, and the last one does so while looking random. This is the
// single most dangerous interface in the library to implement casually.
type CodeGenerator interface {
    Generate(ctx context.Context) (Code, error)
}

// NewRandomGenerator returns the default generator: rejection sampling over
// crypto/rand, so every rune of the alphabet is equally likely (a modulo
// reduction would bias the low runes and shrink the effective keyspace).
func NewRandomGenerator(a Alphabet, length int) (CodeGenerator, error)
```

### 3.4 Policy

```go
type Decision int

const (
    Deny         Decision = iota // reject at creation
    Interstitial                 // create, but warn the visitor (FR-13)
    Allow
)

// Policy classifies a destination. It receives a context because a policy may
// resolve names (SR-07).
//
// The zero-value behaviour of a Policy is never Allow: an implementation that
// cannot reach a resolver must return Deny with a reason, not fall through
// (SR-20).
type Policy interface {
    Evaluate(ctx context.Context, d Destination) (Decision, RejectReason, error)
}

type RejectReason string

const (
    ReasonScheme          RejectReason = "scheme_not_allowed"       // SR-05
    ReasonUserinfo        RejectReason = "credentials_in_url"       // SR-06
    ReasonPrivateAddress  RejectReason = "private_or_internal_host" // SR-07
    ReasonTooLong         RejectReason = "destination_too_long"     // SR-08
    ReasonOwnDomain       RejectReason = "own_domain"               // SR-09
    ReasonControlChars    RejectReason = "control_characters"       // SR-10
    ReasonResolveFailed   RejectReason = "host_resolution_failed"
    ReasonNotAllowlisted  RejectReason = "outside_allowlist"        // → Interstitial
)
```

Implementations in `policy/`:

```go
package policy

func Default(ownDomains []string) cairn.Policy   // SR-05..SR-10, deny-on-error
func Chain(ps ...cairn.Policy) cairn.Policy      // first non-Allow wins
func SchemeAllowlist(schemes ...string) cairn.Policy
func BlockPrivateNetworks(r Resolver) cairn.Policy
func BlockOwnDomains(domains ...string) cairn.Policy
func Allowlist(domains ...string) cairn.Policy   // non-members → Interstitial
func MaxLength(n int) cairn.Policy

// Resolver is net.DefaultResolver in production and a fake in tests. Injecting
// it is what makes SR-07 testable without a network.
type Resolver interface {
    LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}
```

`Chain` composes with **first non-Allow wins**, and `Deny` beats `Interstitial`,
so ordering cannot accidentally downgrade a rejection into a warning.

### 3.5 Hooks

```go
// Hooks receive lifecycle events. Every field may be nil; nil is a no-op.
//
// Handlers are called synchronously and must not block: cairn holds no
// goroutine pool to absorb a slow hook, and a hook that blocks Resolve blocks
// a redirect. Route them into crier from the host (IR-02, ADR-0016).
type Hooks struct {
    OnCreate  func(ctx context.Context, ev CreateEvent)
    OnResolve func(ctx context.Context, ev ResolveEvent)
    OnReject  func(ctx context.Context, ev RejectEvent)
    OnRetry   func(ctx context.Context, ev RetryEvent)
}
```

Event structs carry `Destination` (which redacts itself, so a hook that logs the
whole event still cannot leak — SR-15) and `RejectReason`, never a raw URL
string.

---

## 4. The Shortener

```go
type Shortener struct{ /* unexported */ }

// New validates the configuration eagerly. It fails if no Policy is set, if the
// alphabet and length give a keyspace that is unsafe for the declared expected
// link count, if deduplication is enabled against a store that is not a
// DestIndex, or if the vanity length range overlaps the generated length
// (NFR-08, SR-04, ADR-0002).
func New(store Store, opts ...Option) (*Shortener, error)

func (s *Shortener) Create(ctx context.Context, rawURL string, opts ...CreateOption) (*Link, error)
func (s *Shortener) Resolve(ctx context.Context, c Code) (*Link, error)
func (s *Shortener) Revoke(ctx context.Context, c Code) error
func (s *Shortener) ListByOwner(ctx context.Context, ownerID string, after Cursor, limit int) ([]*Link, Cursor, error)
```

### 4.1 Options

```go
func WithPolicy(p Policy) Option              // required
func WithGenerator(g CodeGenerator) Option
func WithAlphabet(a Alphabet) Option          // default AlphabetBase62
func WithCodeLength(n int) Option             // default 10 — ADR-0002
func WithExpectedLinks(n int64) Option        // feeds the density check at New
func WithMaxSaveAttempts(n int) Option        // default 5 — SR-19
func WithDefaultTTL(d time.Duration) Option
func WithExpiryGrace(d time.Duration) Option  // default 30d — ADR-0009
func WithVanity(minLen, maxLen int, reserved []string) Option
func WithDeduplication(enabled bool) Option   // default false — SR-17
func WithCounter(c Counter) Option
func WithHooks(h Hooks) Option
func WithClock(now func() time.Time) Option   // tests only; no global clock
```

```go
func WithOwner(id string) CreateOption
func WithVanityCode(c Code) CreateOption
func WithTTL(d time.Duration) CreateOption
func WithExpiresAt(t time.Time) CreateOption
```

---

## 5. Flows

### 5.1 Create

```
Create(ctx, rawURL, opts…)
 │
 ├─ 1. length check on the raw string            SR-08   ← before parsing
 ├─ 2. ParseDestination                          SR-05 SR-06 SR-10
 │       scheme allowlist · no userinfo · no control chars
 ├─ 3. normalize (semantics-preserving only)     FR-16   ADR-0015
 ├─ 4. Policy.Evaluate                           SR-07 SR-09
 │       Deny         → ErrDestinationRejected(reason) ─┐
 │       Interstitial → continue, mark on the record    │
 │       Allow        → continue                        │
 ├─ 5. if dedup enabled: DestIndex.LookupByDest         │  SR-17 (off by default)
 │       hit → return the existing Link                 │
 ├─ 6. code:                                            │
 │     ├ vanity given:  alphabet-validate · reserved-word ·
 │     │                length ≠ generated length        SR-04 SR-21
 │     └ generated:     CodeGenerator.Generate           SR-01
 ├─ 7. Store.Save  (conditional, SetNX)                  SR-18
 │       ErrCodeExists → vanity: return it (the caller chose the code)
 │                     → generated: retry from 6, bounded by MaxSaveAttempts
 │                       exhausted → ErrCodeSpaceExhausted     SR-19
 │       store down    → ErrStoreUnavailable, never a success  SR-20
 ├─ 8. if dedup enabled: DestIndex.IndexDest (best-effort, after Save)
 └─ 9. Hooks.OnCreate / Hooks.OnReject ◄─────────────────┘
```

Two orderings in step 7 are load-bearing. **Save before indexing for dedup**: a
dangling index entry is recoverable (the next lookup misses and a new link is
created), while an index entry pointing at a link that was never saved would
resolve to nothing. And **retry only for generated codes**: retrying a vanity
code would mean silently issuing a different code than the caller asked for.

### 5.2 Resolve

```
Resolve(ctx, code)
 │
 ├─ 1. Alphabet.Validate(code) + length bounds   SR-21   ← BEFORE key construction
 │       invalid → ErrInvalidCode, no store round trip
 ├─ 2. Store.Load                                        (namespaced key, SR-22)
 │       miss       → ErrCodeNotFound
 │       store down → ErrStoreUnavailable, never a redirect  SR-20
 ├─ 3. RevokedAt set     → ErrLinkRevoked
 ├─ 4. ExpiresAt passed  → ErrLinkExpired               ADR-0009
 └─ 5. Hooks.OnResolve → return Link
```

Step 1 before step 2 is the whole of SR-21, and it also makes a scanner's
malformed probes free: they never reach the store.

### 5.3 Redirect (cairnhttp)

```
GET /{code}
 │
 ├─ method not GET/HEAD → 405                                 SR-13
 ├─ Shortener.Resolve
 │    ErrInvalidCode | ErrCodeNotFound            → 404 via ErrorEncoder (default)
 │    ErrLinkRevoked | ErrLinkExpired             → 404 via ErrorEncoder (default;
 │                                                   a host may opt into 410 — SR-03)
 │    ErrStoreUnavailable                         → 503, never a redirect  SR-20
 ├─ Interstitial link and interstitial configured → render warning page
 │    continuation control references the CODE only, never a URL       SR-24
 └─ otherwise
      302 Found                                                        SR-11
      Location: <destination>
      Cache-Control: no-store                                          SR-12
      Referrer-Policy: no-referrer                                     SR-14
 │
 └─ after the response is written: Counter.Count on a detached context  FR-14
```

The counter's context is `context.WithoutCancel(ctx)` with its own timeout. The
request context is cancelled the moment the response completes, so passing it
would make every count race the redirect it is counting — the kind of bug that
shows up as "the counter works in tests" (ADR-0012).

### 5.4 Revoke

```
Revoke(ctx, code)
 ├─ 1. Alphabet.Validate                                  SR-21
 ├─ 2. Store.Revoke — sets RevokedAt, keeps the record
 └─ 3. next Resolve fails at step 3, with nothing to invalidate anywhere  SR-23
```

Revocation writes a tombstone rather than deleting, so a revoked link stays
distinguishable from one that never existed for as long as the record lives —
which is what a support conversation needs, and what an audit trail needs.

---

## 6. Store key schema (redisstore)

```
cairn:v1:link:{code}      HASH   the record       TTL = ExpiresAt + grace
cairn:v1:owner:{ownerID}  ZSET   code → CreatedAt (score), for OwnerLister
cairn:v1:dest:{sha256}    STRING code             only when dedup is on
```

- `cairn:` — namespace, so cairn can share an instance without colliding (SR-22).
- `v1` — record schema version, so the format can migrate.
- `{code}` reaches this line already validated against the alphabet (SR-21).
- The dedup index keys on `sha256(normalized destination)`, not the URL, so the
  destination never appears in a key name — key names show up in `SCAN` output,
  `MONITOR`, and slow-query logs, all of which are outside the redaction surface
  of SR-15.

Multi-key writes (record + owner index) go through a single Lua script loaded by
`go:embed` from a `.lua` file (NFR-11), so the record and its index cannot
diverge under a partial failure.

---

## 7. What lives where

| Concern | cairn core | cairnhttp | policy/ | redisstore | Host (moat/crier) |
| --- | :---: | :---: | :---: | :---: | :---: |
| Code generation | ● | | | | |
| Code validation | ● | | | | |
| Destination syntax | ● | | | | |
| Destination policy | interface | | ● | | |
| Lifecycle | ● | | | | |
| Redaction | ● | | | | |
| Persistence | interface | | | ● | |
| Redirect semantics | | ● | | | |
| Error rendering | | plug | | | ● |
| Interstitial page | classify | render | classify | | |
| Rate limiting | | | | | ● moat |
| Auth / ownership check | | | | | ● host |
| Log transport | events | | | | ● crier |
| TLS, routing | | | | | ● host |

The row that matters most is **auth**: cairn carries `OwnerID` as data and never
checks it. `Revoke` does not verify that the caller owns the link — that is the
host's authorization decision, and a library that guessed at it would be wrong in
every deployment that models ownership differently. This is stated in the method
documentation, because it is exactly the assumption a reader is likely to make in
the other direction.
