# Architecture Decision Records

Every structural decision in `cairn` is recorded here (NFR-14). **An ADR is never
edited to reflect a later change of mind.** It is amended in place with a section
naming the ADR that superseded part of it, so the reasoning that was current at
the time stays readable. The convention is inherited from `crier`.

## Index

| ADR | Title | Status | Amended by |
| --- | --- | --- | --- |
| [0001](0001-module-structure-and-dependency-policy.md) | Module structure and dependency policy | Accepted | — |
| [0002](0002-code-generation-strategy.md) | Code generation is CSPRNG-drawn, and length is a security parameter | Accepted | — |
| [0003](0003-feistel-considered-and-deferred.md) | Format-preserving (Feistel) generation — considered, deferred | Accepted — not in v1, reopening criterion recorded | — |
| [0004](0004-alphabet.md) | base62 by default, unambiguous alphabet available, entropy floor preserved | Accepted | — |
| [0005](0005-destination-policy-and-the-ssrf-boundary.md) | Destination policy is required, composable, and honest about SSRF | Accepted | — |
| [0006](0006-redirect-status-and-caching.md) | 302 with explicit `no-store`; never 301 | Accepted | — |
| [0007](0007-destination-redaction-reuses-moat-secret.md) | `Destination` is built on `moat/secret`; the core takes that dependency | Accepted — objection and reopening criterion recorded | — |
| [0008](0008-store-contract-and-durability.md) | One `Store`, conditional writes, fail-closed, durability as a published contract | Accepted | — |
| [0009](0009-lifecycle-logical-expiry-with-a-tombstone.md) | Logical expiry with a grace window; revocation writes a tombstone | Accepted | — |
| [0010](0010-ownership-is-first-class-data-never-authorization.md) | `OwnerID` is first-class data, and never an authorization decision | Accepted | — |
| [0011](0011-vanity-codes-share-one-keyspace-separated-by-length.md) | Vanity codes share the generated keyspace and are separated by length | Accepted | — |
| [0012](0012-click-counting-is-segregated-and-best-effort.md) | Click counting is a segregated, best-effort interface off the hot path | Accepted | — |
| [0013](0013-deduplication-is-opt-in-and-off-by-default.md) | Deduplication is opt-in and off by default | Accepted | — |
| [0014](0014-interstitial-classification-in-core-rendering-in-cairnhttp.md) | The core classifies; `cairnhttp` renders the interstitial | Accepted | — |
| [0015](0015-url-normalization-is-minimal.md) | URL normalization is minimal and semantics-preserving | Accepted | — |
| [0016](0016-observability-through-hooks.md) | Observability is a `Hooks` struct, not an OpenTelemetry dependency | Accepted | — |
| [0017](0017-conventions-are-enforced-not-documented.md) | Conventions are enforced mechanically, or they are not conventions | Accepted | — |

ADR-0001 through ADR-0016 come from the opening design phase; ADR-0017 came
from building the pipeline that has to hold them, and is the only one so far
that is about this repository rather than about the library. ADR-0002 and ADR-0003 are a
pair and should be read together: the first decides what is built, the second
records the alternative that was strong enough to deserve an argument rather than
a dismissal. ADR-0007 is the one that breaks a convention the other two libraries
in this ecosystem hold, and it carries the objection that was raised against it
rather than only the conclusion.

## Reopening criteria on the record

Three decisions carry an explicit trigger, so that "revisit later" is not a
synonym for never:

| ADR | Reopen when |
| --- | --- |
| [0003](0003-feistel-considered-and-deferred.md) | Retry rate exceeds 1% of creations for a week **and** a reset-proof durable sequence exists |
| [0007](0007-destination-redaction-reuses-moat-secret.md) | `moat` raises its Go floor unhelpfully, or the unmask exceeds 1% of redirect latency, or a second breaking minor lands |
| [0008](0008-store-contract-and-durability.md) | The store round trip is *measured* as the redirect bottleneck — and the proposal answers all four listed questions in writing first |

## Open questions

Decisions deferred deliberately, listed here rather than left implicit, because
an undecided question that looks decided is the one that gets implemented by
accident.

1. **IDN destinations.** ADR-0015 rejects non-ASCII hostnames in v1 because
   punycode conversion needs `x/net/idna`, which the core may not import
   (ADR-0001). If a consumer needs IDN, the answer is a satellite module. No
   consumer needs it yet.
2. **Whether `cairnhttp` becomes its own module.** Only becomes live if it grows
   a dependency; it uses `html/template` today (ADR-0001).
3. **A `Store` implementation over Postgres.** Wanted by anyone who reads
   ADR-0008 and does not want the operational contract. Not v1; the interface is
   small enough that it is a consumer-side afternoon.

None of these is blocking v1.
