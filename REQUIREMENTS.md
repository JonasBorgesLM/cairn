# cairn — Requirements

**Version:** 0.1
**Status:** baseline for implementation — no code written yet
**Verified against:** `moat` core (`go 1.24`, no external requires), `crier` core
(`go 1.24`), `usher` requirements v0.3

This document is binding. Every `FR-`, `SR-`, `NFR-` and `IR-` identifier below
is referenced from commit messages, ADRs and tests. **A requirement without a
test that fails when the protection is removed counts as unimplemented** — the
rule is inherited from `usher` and from `crier`'s "a check must fail when it
cannot run".

---

## 1. Purpose

`cairn` is a reusable Go library for URL shortening in which security is a
first-class requirement rather than an afterthought. The name is the stack of
stones that marks the path for a traveller.

It is the third library in a coherent toolkit:

| Library | Concern |
| --- | --- |
| [`moat`](https://github.com/JonasBorgesLM/moat) | HTTP security middleware — rate limiting, CSRF, headers, validation, redaction primitives |
| [`crier`](https://github.com/JonasBorgesLM/crier) | Log control and export to observability backends |
| **`cairn`** | **Short-link generation, resolution, policy and lifecycle** |

Its first real consumer is `task-api` (Handler → Service → Repository, Bearer +
httpOnly cookie dual-mode auth, standardized JSON error envelope, single-instance
Kubernetes deploy).

### 1.1 What cairn is responsible for

- Generation and resolution of short codes.
- Destination validation policy.
- Storage abstraction.
- Link lifecycle: expiry, revocation, ownership.

### 1.2 Non-goals — stated explicitly to bound the scope

- **HTTP routing, authentication, authorization.** The core must not import
  `net/http` and must not have an opinion about who may shorten. The optional
  `cairnhttp` package is a convenience handler, not the product.
- **Rate limiting.** That is `moat`'s (IR-01). cairn depends on it being applied
  by the host and says so in the places where its own safety assumes it
  (SR-16, SR-19).
- **Observability transport.** That is `crier`'s (IR-02). cairn emits events
  through a `Hooks` struct and never opens a connection (NFR-06, ADR-0016).
- **Product analytics.** Click counting exists only as the narrow, segregated,
  best-effort counter of FR-14 — not as an analytics pipeline.
- **A hosted service.** cairn is a library. There is no `cairnd`.

---

## 2. Actors

| Actor | Role |
| --- | --- |
| Host application | Imports cairn; owns routing, auth, rate limiting, TLS |
| Creator | Authenticated principal of the host who creates a short link |
| Visitor | Anyone who resolves a short code; unauthenticated by assumption |
| Store operator | Runs the Redis (or other) backend cairn writes to |
| Scanner | Unauthenticated party enumerating the code space |
| Abuser | Party using the shortener to launder a phishing or SSRF destination |

The last two are not edge cases. They are the reason this library exists in the
shape it does, and they are modelled in [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md).

---

## 3. Functional requirements

- **FR-01** Create a short link from a destination URL, returning a `Link` whose
  `Code` is unique within the store.
- **FR-02** Resolve a code to its destination, distinguishing *not found*,
  *expired* and *revoked* as separate outcomes (FR-09).
- **FR-03** Revoke a link, so that subsequent resolution fails immediately and
  unconditionally.
- **FR-04** Abstract storage behind a `Store` interface, so the same core runs
  against Redis, an in-memory implementation, or a consumer's own backend
  (ADR-0008).
- **FR-05** Ship `memstore`, an in-memory `Store`, so consumers can test their
  own integration without a container.
- **FR-06** Ship `redisstore` as a separate module (go-redis), so the core's
  dependency set does not grow for consumers who do not use Redis.
- **FR-07** Abstract code generation behind a `CodeGenerator` interface, with a
  CSPRNG-backed generator as the default (SR-01, ADR-0002).
- **FR-08** Expose `Alphabet` as a validated type, with `AlphabetBase62` as the
  default and `AlphabetUnambiguous` provided for transcription contexts
  (ADR-0004).
- **FR-09** Support logical expiry: `Link.ExpiresAt` is recorded in the stored
  record, and an expired-but-not-yet-collected link resolves to `ErrLinkExpired`,
  which is distinguishable from `ErrCodeNotFound` (ADR-0009).
- **FR-10** Carry `Link.OwnerID` as a first-class field, so a host can list and
  revoke by owner without maintaining a parallel index (ADR-0010).
- **FR-11** Support vanity (caller-chosen) codes, in the same keyspace as
  generated codes and separated from them by length (ADR-0011).
- **FR-12** Provide `cairnhttp`, an optional redirect handler with a pluggable
  `ErrorEncoder`, so the host keeps its own error envelope (IR-04).
- **FR-13** Classify a destination as `Allow`, `Interstitial` or `Deny`, and let
  `cairnhttp` render an interstitial warning page for the middle case
  (ADR-0014).
- **FR-14** Support click counting through a `Counter` interface segregated from
  `Store`, invoked off the resolve critical path and best-effort (ADR-0012).
- **FR-15** Support deduplication (same destination → same code) as an
  explicitly opt-in, off-by-default option (ADR-0013).
- **FR-16** Normalize destination URLs only where RFC 3986 declares the forms
  equivalent, never where a server may distinguish them (ADR-0015).
- **FR-17** Emit lifecycle events through a `Hooks` struct with no-op defaults
  (ADR-0016).

---

## 4. Security requirements

These are the reason for the library. Each maps to a threat in
[`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md).

### 4.1 Code generation

- **SR-01 Unpredictability.** Codes are drawn from `crypto/rand`, never from a
  counter, a timestamp, a PRNG seeded from either, or a hash of the destination.
  Holding any set of issued codes must give no advantage in predicting the next
  one.
- **SR-02 Scan resistance.** *A distinct property from SR-01, covered by a
  distinct mechanism.* Unpredictability says the next code cannot be guessed
  from previous ones; scan resistance says the space cannot be swept. It is a
  function of **keyspace density**, not of randomness: for `N` live links, an
  alphabet of size `A` and length `L`, the expected number of hits for an
  attacker issuing `R` requests is

  ```
  E[hits] = R · N / A^L
  ```

  The default length is chosen so that `E[hits]` stays negligible at realistic
  `N` and `R` (ADR-0002). Rate limiting reduces `R` but is the host's job via
  `moat` (IR-01) — cairn must remain safe against a scanner who is not rate
  limited, only slower.
- **SR-03 Uniform rejection.** A resolve miss must be indistinguishable in
  status, body shape and timing from a resolve of a code that is revoked or
  expired *at the transport level*, unless the host explicitly opts into
  distinguishing them. FR-09's distinction is available to the host as typed
  errors; it is not automatically leaked to the visitor.
- **SR-04 Vanity codes never occupy the generated length.** A vanity code whose
  length equals the configured generated length is rejected. This prevents both
  squatting on future generated codes and using vanity creation as a probe of
  the generated space (ADR-0011).

### 4.2 Destination policy

- **SR-05 Scheme allowlist.** Only `http` and `https`. Everything else —
  `javascript:`, `data:`, `file:`, `ftp:`, custom app schemes — is rejected. The
  list is an allowlist, never a denylist.
- **SR-06 No embedded credentials.** A URL carrying userinfo
  (`https://user:pass@host/`) is rejected. It is a phishing primitive and a
  credential leak into every log and Referer.
- **SR-07 No internal destinations.** Reject destinations that resolve to
  loopback, RFC 1918, link-local (including `169.254.169.254`), CGNAT
  `100.64.0.0/10`, unspecified, multicast, IPv6 unique-local and IPv4-mapped
  IPv6 forms, and reject non-decimal IPv4 literals (`0x7f.1`, `2130706433`,
  `0177.0.0.1`) rather than trying to parse them.

  **Threat model, stated because the mitigation is partial.** cairn does not
  fetch destinations, so this does not protect cairn. It protects *third-party
  services that follow redirects*: an attacker who cannot reach
  `http://169.254.169.254/` through such a service's own allowlist shortens it,
  and passes the allowlisted short domain instead. The shortener would otherwise
  be a general-purpose allowlist bypass.

  **Known limit, not a defect to be hidden:** the check binds a hostname to an
  address at *create* time, and DNS may answer differently at *resolve* time.
  This is a TOCTOU gap that no create-time check can close. The complete
  mitigation lives in the fetching service, which must re-validate the address
  it actually connects to. SR-07 raises the cost; it does not remove the class.
  This limitation is stated in the package documentation, not only here.
- **SR-08 Destination length limit.** A conservative default maximum (2000
  bytes), enforced before parsing.
- **SR-09 Anti-loop.** Reject destinations on the service's own configured
  domain(s). Chained third-party shorteners are explicitly *not* addressed —
  maintaining a denylist of shortener domains is a losing game, and pretending
  otherwise would be worse than saying so.
- **SR-10 No control characters or whitespace** anywhere in the destination,
  including percent-decoded forms in the host.

### 4.3 Redirect behaviour

- **SR-11 Never 301.** A cached permanent redirect survives revocation in every
  intermediary and browser that holds it, so revocation would be an illusion.
  `cairnhttp` answers `302 Found`.
- **SR-12 Explicit no-store.** Choosing 302 is not sufficient: RFC 9111 §4.2.2
  permits heuristic caching of a 302 in the absence of explicit directives. The
  redirect response must carry `Cache-Control: no-store` (ADR-0006).
- **SR-13 GET and HEAD only** on the redirect route; other methods get `405`,
  which removes the reason to reach for 307/308.
- **SR-14 No Referer leak of the short code to the destination** —
  `Referrer-Policy: no-referrer` on the redirect response.

### 4.4 Data handling

- **SR-15 Destinations redact by default.** `Destination` must write a redacted
  form through every formatting, logging and marshalling path the standard
  library offers (`String`, `Format`, `GoString`, `LogValue`, `MarshalText`,
  `MarshalJSON`). URLs routinely carry tokens and PII in the query string, and
  the leak is never a deliberate print — it is a `%+v` while debugging
  (ADR-0007).
- **SR-16 Vanity availability is an oracle, and is treated as one.** Telling a
  caller that a vanity code is taken is unavoidable — they need to know. The
  mitigations are: creation is authenticated (the host's job), rate limited via
  `moat` (IR-01), and answered in constant shape. cairn documents the oracle
  rather than pretending it closed it.
- **SR-17 Deduplication is off by default** because it is an existence oracle:
  with it on, submitting a URL and receiving an existing code reveals that
  someone already shortened it (ADR-0013).

### 4.5 Storage

- **SR-18 Conditional write only.** `Save` uses `SetNX` semantics. A collision
  returns `ErrCodeExists` and drives a bounded retry. An unconditional `SET`
  would silently repoint an existing link at an attacker's destination — the
  single worst failure this library can have.
- **SR-19 Bounded retry.** Retry on `ErrCodeExists` has a hard ceiling; on
  exhaustion `Create` fails. An unbounded retry loop under a saturated keyspace
  is a denial of service on the creation path.
- **SR-20 Fail-closed.** With the store unavailable, resolution returns an error
  and never a redirect, and creation returns an error and never a success. There
  is no configuration that makes this fail-open. Coherent with `moat`'s
  rate-limit posture.
- **SR-21 Validate the code before building the key.** A code is checked against
  the alphabet and the length bounds *before* it is concatenated into the store's
  keyspace, so a hostile code can never address a key outside the namespace or
  collide with an unrelated one.
- **SR-22 Namespaced, versioned keys.** All keys carry a fixed prefix and a
  schema version, so cairn can share a Redis instance without colliding with a
  host's own keys and can migrate its record format.
- **SR-23 Revocation beats caching, by construction.** There is no read cache in
  front of the store in v1, so a revoked link cannot be served from a stale copy
  (ADR-0008). Adding one later requires an ADR that answers the staleness
  window explicitly.
- **SR-24 Interstitial continuation references the code, never a URL.** An
  interstitial page whose "continue" control takes a destination parameter *is
  itself an open redirect*. The control references the short code only
  (ADR-0014).

---

## 5. Non-functional requirements

- **NFR-01 Dependency policy.** The core module's only non-standard-library
  dependency is `github.com/JonasBorgesLM/moat`, a first-party module, pinned to
  an exact version and never auto-upgraded (ADR-0001, ADR-0007). No third-party
  dependency enters the core. `redisstore` carries `go-redis` and its test
  dependencies; that is what a satellite module is for.
- **NFR-02 Go 1.24 is the floor in the core**, matching `moat` core and `crier`
  core. The floor is the *lowest viable* version, because the `go` directive in
  a library is a compatibility promise about who may import it. It moves only
  when something concrete makes it non-viable, and that reasoning is written as
  an ADR before the directive changes. Satellite modules may declare a higher
  floor when a dependency imposes one — and that is documented as an imposition,
  not a choice.
- **NFR-03 Multi-module repository.** `cairn` (core) and `cairn/redisstore` are
  independently versioned modules with independent tags. A green build in one
  says nothing about the other; every command runs per module.
- **NFR-04 No global state.** No package-level mutable configuration, no
  `init()` that registers anything, no singleton store.
- **NFR-05 `context.Context` on every I/O operation**, first parameter, never
  stored in a struct.
- **NFR-06 No transport in the core.** The core does not import `net/http`, does
  not open sockets, and does not depend on an observability SDK.
- **NFR-07 Sentinel errors comparable with `errors.Is`**, and no leaking of
  `go-redis` types through any public API — a `redisstore` failure surfaces as a
  cairn error wrapping the cause.
- **NFR-08 Functional options** for all configuration, validated eagerly at
  construction. An invalid configuration fails at `New`, not at first use.
- **NFR-09 Testable examples (`ExampleXxx`) in every public package**, matching
  `moat` and `crier`.
- **NFR-10 Integration tests use testcontainers against a real Redis**, never
  miniredis — a fake that implements `SetNX` correctly proves nothing about the
  server that has to.
- **NFR-11 Lua scripts, if any, live in a separate `.lua` file** loaded with
  `go:embed`, never as a Go string literal.
- **NFR-12 English** for all code, comments, documentation and commit messages,
  regardless of the language a request was written in.
- **NFR-13 Conventional Commits and semantic versioning from v0.1.0.** One
  subject per commit, staged explicitly.
- **NFR-14 Every structural decision is an ADR** under `docs/adr/`. An ADR is
  never rewritten — it is amended in place or superseded by a new one that names
  it.
- **NFR-15 CI runs** `go build`, `go vet`, `go test -race`, `golangci-lint`,
  `govulncheck` and the integration suite, **per module**.
- **NFR-16 Branching.** Feature branches merge to `develop` via PR; `main` is the
  release branch.
- **NFR-17 The threat model is exercised by something re-runnable**, and the
  result is recorded — following `crier`'s `crier/IR-06`. A threat model nobody has
  attacked is a wish list.

---

## 6. Ecosystem integration requirements

- **IR-01 `moat` covers the edge.** Rate limiting on both the create and the
  resolve routes, security headers, request size limits and CSRF on
  browser-driven creation are composed by the host from `moat`. cairn names the
  properties it depends on; it does not re-implement them (see
  [`docs/INTEGRATION.md`](docs/INTEGRATION.md)).
- **IR-02 `crier` covers log transport.** cairn's `Hooks` produce structured
  values; the host routes them into `crier`. cairn never imports an exporter.
- **IR-03 `moat/secret` covers redaction.** `Destination` is built on
  `secret.Value` (ADR-0007). This is the one first-party dependency in the core.
- **IR-04 `task-api` is the first real consumer**, and the integration is a
  deliverable, not a possibility: a documented handler, a repository binding, and
  the error envelope mapped through `cairnhttp.ErrorEncoder`.
- **IR-05 A runnable demo** — `docker-compose` with Redis, a minimal host
  composing `moat` + `cairn`, and the threat probes of NFR-17 pointed at it.

---

## 7. Out of scope for v1

Recorded here so that a deferred decision does not look like a decided one.

- A layered durable-store + cache topology (ADR-0008 keeps the door open and
  names what would have to be answered).
- Feistel / format-preserving code generation (ADR-0003 records the analysis and
  the reopening criterion).
- Bulk creation, batch resolution, link editing (a link's destination is
  immutable; changing it means creating a new one).
- Analytics beyond FR-14's counter.
- Any second `Store` implementation beyond `memstore` and `redisstore`.

---

## 8. v1 acceptance

v0.1.0 is complete when:

1. A host can compose `moat` + `cairn` + `redisstore`, create a link, resolve it,
   revoke it, and observe the revocation take effect on the next request with no
   cache to wait out.
2. Every `SR-` identifier is covered by at least one test that has been **seen to
   fail** with the protection removed — a negative control, per `crier`'s rule.
3. `docs/adr/README.md` lists no open question that the code has already answered
   by accident.
4. The NFR-17 probe script runs against the IR-05 demo and its output is recorded
   in the repository.
