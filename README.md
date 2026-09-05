# cairn

A Go library for URL shortening in which security is a requirement, not an
addendum. The name is the stack of stones that marks the path for a traveller.

> **Status: pre-implementation.** Requirements, threat model, architecture and
> seventeen ADRs are written, and the pipeline that enforces them is running.
> The domain code is not written. See [Roadmap](#roadmap) — nothing here is
> importable yet.

---

## Why this exists

Most shortener libraries answer "how do I make a short code" and stop. The
interesting part starts after that: what happens when someone enumerates the code
space, when a cached 301 outlives the revocation that was supposed to kill it,
when a shortened `http://169.254.169.254/` becomes an allowlist bypass into
somebody else's metadata endpoint, when a URL carrying a password-reset token
lands in a log index with a two-year retention.

`cairn` takes positions on those, and **writes down the reasoning** so it can be
inspected rather than trusted:

- [`REQUIREMENTS.md`](REQUIREMENTS.md) — traceable `FR-`/`SR-`/`NFR-`/`IR-` ids
- [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md) — fifteen threats, their
  mitigations, and **what is left over**
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — packages, proposed signatures,
  create/resolve flows
- [`docs/adr/`](docs/adr/README.md) — the decision record
- [`docs/INTEGRATION.md`](docs/INTEGRATION.md) — the boundary with `moat`,
  `crier` and `task-api`
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — git flow, commit convention, and the
  documentation rules CI enforces
- [`RELEASING.md`](RELEASING.md) — why the tag order is not optional

## Security properties

| Property | How |
| --- | --- |
| Codes are unpredictable | `crypto/rand` with rejection sampling; no counter, no timestamp, no hash of the destination |
| The space cannot be swept | Length is a security parameter, not an aesthetic one: `E[hits] = R·N/A^L`, default 10 base62 runes ≈ 59.5 bits ([ADR-0002](docs/adr/0002-code-generation-strategy.md)) |
| Revocation actually revokes | Never 301; 302 **plus** explicit `no-store`, and no read cache to serve a stale copy ([ADR-0006](docs/adr/0006-redirect-status-and-caching.md), [ADR-0008](docs/adr/0008-store-contract-and-durability.md)) |
| A link is never repointed | Conditional write only. There is no code path that overwrites an existing code ([ADR-0008](docs/adr/0008-store-contract-and-durability.md)) |
| Destinations do not leak into logs | `Destination` redacts through `String`, `Format`, `LogValue`, `MarshalJSON` and every other path the standard library offers ([ADR-0007](docs/adr/0007-destination-redaction-reuses-moat-secret.md)) |
| Internal addresses are refused | Loopback, RFC 1918, link-local, CGNAT, IPv6 ULA, IPv4-mapped IPv6, and non-decimal IPv4 literals ([ADR-0005](docs/adr/0005-destination-policy-and-the-ssrf-boundary.md)) |
| Vanity codes cannot probe the generated space | A vanity code may not be the generated length ([ADR-0011](docs/adr/0011-vanity-codes-share-one-keyspace-separated-by-length.md)) |
| The store going down is not permission | Fail-closed on both paths, not configurable ([ADR-0008](docs/adr/0008-store-contract-and-durability.md)) |

### And what it does not do

Stated here rather than in a footnote, because a security list that mentions only
wins is a sales page:

- **It does not stop phishing.** A well-formed `https://` destination is
  indistinguishable from a legitimate one to any library. cairn removes the
  mechanical laundering primitives — `javascript:`, embedded credentials, its own
  domain — and offers an interstitial. No reputation feed, no content scanning.
- **It does not close SSRF.** Hostnames are checked at *create* time; DNS can
  answer differently at *resolve* time. That TOCTOU gap is unclosable from here,
  and the complete mitigation belongs to whatever service follows the redirect.
- **It does not do rate limiting, auth, or log shipping.** Those are
  [`moat`](https://github.com/JonasBorgesLM/moat), the host, and
  [`crier`](https://github.com/JonasBorgesLM/crier).

Full residual analysis: [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md) §7.

## Design shape

```
cairn/            core — Shortener, Link, Destination, Code, Policy interface
├── policy/       destination policy implementations
├── memstore/     in-memory Store for consumer tests
└── cairnhttp/    optional redirect handler, pluggable ErrorEncoder

cairn/redisstore/ separate module — go-redis, testcontainers
```

The core does not import `net/http` and has no opinion about who may shorten.
Its one non-standard-library dependency is `moat`, first-party and pinned exactly
— the reasoning, and the objection raised against it, are in
[ADR-0007](docs/adr/0007-destination-redaction-reuses-moat-secret.md).

## Operational contract

Redis is the single source of truth. Losing the dataset breaks every link ever
issued, permanently — including the ones already printed on paper. These are
requirements of the library, not suggestions
([ADR-0008](docs/adr/0008-store-contract-and-durability.md)):

- **AOF with `appendfsync everysec`.** RDB-only snapshots lose everything created
  since the last save.
- **`maxmemory-policy noeviction`.** Any `allkeys-*` policy silently deletes
  links under memory pressure. `redisstore` verifies this at startup and refuses
  to run without it.
- **A replica, and a restore that has actually been exercised.** A backup nobody
  has restored is a hope.
- **A dedicated database index.** cairn's keys are namespaced; `FLUSHDB` is not.

Resolution availability equals store availability. That is the accepted cost of
having no cache that could serve a revoked link.

## Roadmap

| Milestone | Delivery |
| --- | --- |
| **M0** Foundation | Documentation, module skeletons, CI/CD, linting, conventions *(done)* |
| **M1** Core types | `Code`, `Alphabet`, `Destination`, `Link`, errors, `Hooks` *(current)* |
| **M2** Generation | `CodeGenerator`, CSPRNG generator, density validation |
| **M3** Policy | `Policy`, `policy/` implementations, the SSRF boundary |
| **M4** Shortener + memstore | Create / Resolve / Revoke, retry, `memstore` |
| **M5** redisstore | go-redis store, Lua scripts, testcontainers, eviction check |
| **M6** cairnhttp | Redirect handler, `ErrorEncoder`, interstitial |
| **M7** Extras | Vanity, ownership listing, counter, deduplication |
| **M8** Verification | Threat probes, benchmarks, negative-control audit |
| **M9** Release | `v0.1.0` and `redisstore/v0.1.0` |

## Ecosystem

| Library | Concern |
| --- | --- |
| [`moat`](https://github.com/JonasBorgesLM/moat) | HTTP security middleware |
| [`crier`](https://github.com/JonasBorgesLM/crier) | Log control and export |
| **cairn** | Short links |

## License

[MIT](LICENSE).
