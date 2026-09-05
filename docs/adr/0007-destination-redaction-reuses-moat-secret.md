# ADR-0007: `Destination` is built on `moat/secret`, and the core takes that dependency

## Status
Accepted — with a recorded objection and a reopening criterion

## Context
URLs carry secrets. Password-reset tokens, pre-signed object-storage URLs,
session identifiers, email addresses and account identifiers all routinely live
in a query string. A shortener stores those URLs, passes them through error
paths, and returns them in a `Location` header — and the leak is never a
deliberate print. It is `%+v` on a struct while debugging, a `slog.Any("link", l)`
that looked harmless, an error message interpolating the value it failed to
parse. The value then lives in CI output, container logs, and whatever indexes
them, for the retention period.

`moat`'s `secret.Value` already solves exactly this, and solves it better than a
naive `String()` override: bytes are held XOR-masked under a per-value keystream
derived from a process key, so even a *structural* dump — the reflection-based
paths that no method can intercept — reveals nothing. That construction was
arrived at after a first version with a process-wide positional pad was found to
fail to known-plaintext recovery and pad reuse. Reproducing it here means
reproducing that analysis, and probably reproducing the first version of it.

Against that stands the convention this ecosystem has held twice: `moat` core and
`crier` core both have zero non-standard-library dependencies, enforced in CI.

## Decision
`Destination` is built on `secret.Value`. The core module takes
`github.com/JonasBorgesLM/moat` as a dependency, and the dependency policy is
restated as **first-party dependencies only, pinned exactly** (ADR-0001) rather
than as zero dependencies.

`Destination` holds:
- the raw URL in a `secret.Value`;
- `scheme` and `host` in the clear, because those are what an operator needs to
  triage an incident and neither carries the secret.

The redacted rendering is `https://example.com/[REDACTED]` — authority kept,
everything after it replaced — through `String`, `GoString`, `Format`,
`LogValue`, `MarshalText` and `MarshalJSON`. `URL()` and `Raw()` are the only
ways out, and both are explicit enough to be visible in review.
`UnmarshalText`/`UnmarshalJSON` refuse a value containing the placeholder,
mirroring `secret.ErrRedacted`, so a JSON round trip cannot launder a redacted
rendering back into a live destination.

## The objection, recorded rather than resolved away
This was argued against before it was accepted, and the argument is kept because
the reopening criterion below depends on it:

1. **It breaks the invariant that made the other two libraries easy to adopt.**
   "Zero dependencies" is a property a consumer can verify in one glance at
   `go.mod`. "One first-party dependency" requires them to trust a second
   repository's release discipline.
2. **`moat` is pre-1.0 and independently unaudited.** `usher`'s ADR-11 already
   records that `moat` took a breaking `Store` rename in a *minor* release and
   once published a satellite tag that did not compile. That is not
   disqualifying, but it is the reason for the pinning rule below.
3. **It puts an HMAC-SHA-256 keystream derivation on the resolve hot path.**
   Every redirect unmasks a `Destination` to build a `Location` header. The cost
   is small in absolute terms — one keystream over a URL-length input — but it
   is on the one path that must never become the bottleneck, and it is a cost a
   plain `String()` override would not have.

The counter-argument that carried the decision: the masking is materially
stronger than a method-based redaction, the reflection-reachable paths are
genuinely uncloseable any other way, and a second hand-rolled implementation of a
primitive that took two iterations to get right in `moat` is the worse risk.

## Consequences
- `moat` is pinned to an exact version in the core `go.mod` and never
  auto-upgraded. Upgrades are manual, deliberate, and verified with a build from
  a clean directory outside the repository — the procedure `usher`'s ADR-11
  established.
- Dependabot is configured **not** to open PRs for `moat` in the core module.
  An automated bump of this dependency is precisely the event this ADR exists to
  prevent.
- CI enforces that the core's `go.mod` requires nothing outside
  `github.com/JonasBorgesLM/`. The policy is a check, not a preference.
- Benchmarks cover the unmask on the redirect path, so the cost in objection 3 is
  a measured number rather than a worry.
- `cairn`'s minimum Go version is now bounded below by `moat` core's, which is
  also 1.24 today. If `moat` core raises its floor, cairn's floor moves with it
  whether or not cairn needs it — an imposition, and it gets documented as one.

## Reopening criterion
Revisit if any of these becomes true:
- `moat` core raises its Go floor for a reason cairn does not benefit from, and
  the imposition strands a consumer;
- the benchmark shows the unmask exceeding 1% of redirect-path latency at
  realistic URL lengths;
- `moat` ships a second breaking change in a minor release.

The fallback if reopened is not "hand-roll it": it is the satellite-module shape
— a redacting `Destination` in the core with no dependency, and an optional
`cairn/moatsecret` adapter for consumers who already have `moat` in their graph.
That shape was considered and rejected here only because it splits one type's
behaviour across two modules; if the dependency becomes a liability, that cost
inverts.
