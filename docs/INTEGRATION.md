# cairn — Integration plan

**Version:** 0.1
**Companions:** [`../REQUIREMENTS.md`](../REQUIREMENTS.md) (IR-01…IR-05) ·
[`THREAT-MODEL.md`](THREAT-MODEL.md) · [`ARCHITECTURE.md`](ARCHITECTURE.md)

This document states what each library in the ecosystem covers, **where the
overlaps are**, and who wins in each. An overlap that is not named is one that
gets implemented twice and diverges.

---

## 1. The division of labour

```
   Visitor / Creator
        │
   ┌────▼─────────────────────────────────────────────────┐
   │ moat        ratelimit · secureheaders · realip · csrf │  edge
   ├──────────────────────────────────────────────────────┤
   │ task-api    routing · authn · authz · error envelope  │  host
   ├──────────────────────────────────────────────────────┤
   │ cairn       codes · policy · lifecycle · storage      │  domain
   ├──────────────────────────────────────────────────────┤
   │ crier       log normalization · export                │  observability
   └──────────────────────────────────────────────────────┘
```

Read top to bottom, the rule is: **each layer assumes the one above it did its
job, and none of them re-does it.** Where cairn's own safety depends on a layer
above, the requirement says so by name (SR-02 → IR-01, SR-16 → IR-01) rather
than leaving it as an assumption a deployer can silently break.

---

## 2. `moat` — the edge

### 2.1 What moat provides and cairn still does

| Concern | `moat` provides | cairn still does |
| --- | --- | --- |
| Scan rate limiting (T-01) | `ratelimit.Limiter` on `GET /{code}` | Keyspace density (SR-02) — must hold without rate limiting |
| Create abuse (T-10) | `ratelimit` on `POST /links`, keyed per principal | Bounded retry (SR-19), destination length (SR-08) |
| Vanity probing (T-09) | `ratelimit` on create | Length separation (SR-04), which is the part that actually closes the generated-space probe |
| Client IP for keys | `realip` with a declared trusted-proxy CIDR set | — |
| Response headers | `secureheaders` | Redirect-specific headers on the redirect response only (SR-12, SR-14) |
| Browser-driven creation | `csrf` signed double-submit | — |
| Body size / content type | `validate` | Destination length before parsing (SR-08) |
| Secret redaction | `secret.Value` | `Destination` built on it (ADR-0007) |

### 2.2 The overlaps, named

**Rate limiting is moat's, and cairn does not have a partial version.** cairn
carries no request counter and no backoff. The `moat` limiter is the only one.
Where cairn's threat analysis mentions rate limiting, it is as a *reduction* of
the attacker's `R`, never as the mitigation — SR-02 must hold against a
distributed scanner that defeats per-IP limiting entirely.

**Header setting overlaps for real.** `secureheaders` sets a policy across all
responses; `cairnhttp` sets `Cache-Control: no-store` and
`Referrer-Policy: no-referrer` on the redirect. Both write to the same header
map, and last-writer-wins depends on middleware order. The resolution:
`cairnhttp` sets its headers **immediately before `WriteHeader`**, so a
surrounding middleware cannot overwrite the two headers that revocation and
SR-14 depend on. This is a correctness requirement on `cairnhttp`, tested with a
`secureheaders` chain wrapped around it — a test that would otherwise never be
written, because each component is correct alone.

**`realip` topology, both sides.** If the host runs behind a load balancer, the
trusted-proxy CIDR set must be declared, and an empty set or `0.0.0.0/0` is a
construction error in `moat`. Getting this wrong silently disables the rate
limiting that T-01's residual leans on. Same warning as `usher`'s ADR-10.

**`secret.Value` is a real dependency, not a shared philosophy.** Unlike
`crier`, which followed moat's approach without importing it, cairn imports it
(ADR-0007). The version is pinned exactly and Dependabot is configured not to
bump it.

### 2.3 ADRs cite properties, not the library

Following `usher`: an ADR states *"the rate-limit key derives from the first
untrusted peer, so a forged `X-Forwarded-For` cannot control it"* and notes that
`moat` supplies it today. If the library is replaced, the requirement outlives
the dependency.

---

## 3. `crier` — observability

### 3.1 The boundary

cairn produces **events**; crier moves them. cairn never opens a connection,
never batches, never retries an export, and imports no exporter (NFR-06,
ADR-0016).

```go
// Host-side adapter. cairn's side of this is the Hooks struct and nothing else.
hooks := cairn.Hooks{
    OnReject: func(ctx context.Context, ev cairn.RejectEvent) {
        logger.WarnContext(ctx, "destination rejected",
            "reason", ev.Reason,     // typed RejectReason, never a free string
            "dest",   ev.Dest,       // Destination.LogValue() redacts — SR-15
            "owner",  ev.OwnerID,
        )
    },
}
```

### 3.2 What is safe to send, and why

`ev.Dest` is a `Destination`. Its `LogValue` renders
`https://example.com/[REDACTED]`, so a host that logs the whole event and ships
it to crier cannot leak a token in a query string — the property crier's own
redaction stage (ADR-0006/0014 there) would otherwise have to catch by pattern.

**This is a defence in depth, not a reason to disable crier's redaction.** cairn
guarantees its own types redact; it guarantees nothing about the message the host
writes around them.

### 3.3 The overlap, named

Both libraries have a redaction story. They operate at different layers and
neither replaces the other:

| | cairn | crier |
| --- | --- | --- |
| Unit | the `Destination` **type** | the log **record** |
| Mechanism | masked in memory, redacts through every format path | rule-based masking of attributes and body |
| Covers | anything holding a `Destination`, everywhere | anything matching a rule, at export |
| Fails | never — the type has no unredacted format path | fail-closed: unredactable records are dropped |

cairn's is structural and cannot be forgotten. crier's is rule-based and catches
what cairn never saw. Turning either off because the other exists is the mistake
this table is here to prevent.

---

## 4. `task-api` — the first real consumer (IR-04)

### 4.1 Where cairn lands in the layering

`task-api` is Handler → Service → Repository. cairn is **Service-layer domain
logic with its own Repository**, not a repository itself:

```
Handler   POST /links, GET /links, DELETE /links/{code}, GET /{code}
   │      Bearer or httpOnly cookie; error envelope
Service   authorize; derive OwnerID from the principal; call cairn
   │
cairn     Shortener  ──►  redisstore.Store  ──►  Redis
```

The Handler never touches `Shortener` directly, because the Service is where the
authorization decision lives — and ADR-0010 makes that non-optional: `Revoke`
does **not** check ownership. The Service must load the link, compare
`OwnerID` against the authenticated principal, and only then revoke.

**This is the single most likely integration bug**, and it is the kind that
passes every test written by the person who wrote the code. It gets a dedicated
negative test in `task-api`: user A cannot revoke user B's link.

### 4.2 Error envelope mapping (FR-12)

`task-api` has a standardized JSON error envelope. `cairnhttp.ErrorEncoder` is
the seam:

| cairn error | HTTP | Envelope code | Note |
| --- | --- | --- | --- |
| `ErrInvalidCode` | 404 | `link_not_found` | |
| `ErrCodeNotFound` | 404 | `link_not_found` | |
| `ErrLinkExpired` | 410 for an authenticated request, else 404 | `link_expired` / `link_not_found` | `task-api`'s own opt-in (SR-03) — see below |
| `ErrLinkRevoked` | 410 for an authenticated request, else 404 | `link_revoked` / `link_not_found` | `task-api`'s own opt-in (SR-03) — see below |
| `ErrDestinationRejected` | 422 | `destination_rejected` + `reason` | `RejectReason` is a stable enum, safe to expose |
| `ErrCodeExists` | 409 | `code_taken` | Vanity only |
| `ErrVanityReserved` | 422 | `code_reserved` | |
| `ErrVanityLength` | 422 | `code_length_invalid` | |
| `ErrCodeSpaceExhausted` | 503 | `temporarily_unavailable` | Alert-worthy: the keyspace is saturating |
| `ErrStoreUnavailable` | 503 | `temporarily_unavailable` | Never a redirect (SR-20) |

**The 410-versus-404 decision belongs to `task-api`, not to cairn.**
`cairnhttp.DefaultErrorEncoder` maps not-found, expired and revoked all to the
same 404 by default (SR-03): distinguishing them is useful to a legitimate user
and is an oracle to a scanner, so a host must opt into it rather than get it for
free. `task-api`'s own `ErrorEncoder`, shown above, makes that opt-in choice:
**410 for authenticated requests, 404 for anonymous ones.** The owner of a link
learns why it failed; a scanner learns nothing. That is the whole reason
`ErrorEncoder` is a function rather than a table.

### 4.3 Deployment

Single-instance Kubernetes. From ADR-0008's operational contract, and these are
deployment blockers, not recommendations:

- Redis with AOF `appendfsync everysec` and a PVC that survives a pod restart.
- `maxmemory-policy noeviction` — verified at startup by
  `EvictionChecker`; the pod fails its readiness probe against a misconfigured
  instance rather than starting and losing links later.
- A restore that has been run at least once. A single-instance deployment has no
  replica, so the backup **is** the durability story.
- `moat` rate limiting shares the Redis instance; cairn's keys are namespaced
  (SR-22) but `FLUSHDB` is not. Separate database indices.

### 4.4 Rollout order

1. `task-api` takes cairn with `memstore` behind a feature flag; no Redis yet.
2. Swap to `redisstore` in staging; run the NFR-17 probes against it.
3. Enable creation for a subset of users; watch `Hooks.OnRetry` (ADR-0003's
   criterion needs a baseline before it can be a trigger).
4. Public resolve route; `moat` rate limiting sized from step 3's traffic.

---

## 5. What is not covered by any of the four

Stated so the gap is a decision rather than a discovery:

- **Malicious-but-well-formed destinations** (T-04 residual). No reputation feed
  anywhere in the stack. The interstitial shifts the decision to the visitor.
- **Abuse quotas per creator over time** — `moat` rate limits a window; nothing
  counts "this account has shortened 4,000 URLs this month". That is a
  `task-api` product decision and it does not exist yet.
- **DNS rebinding between create and resolve** (T-05 residual). Belongs to the
  fetching service, which is not in this stack.
- **Takedown workflow.** `Revoke` is the mechanism; the process around it —
  who reports abuse, who decides, what the visitor sees — is `task-api`'s and is
  unbuilt.
