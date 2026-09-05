# ADR-0008: One `Store`, conditional writes, fail-closed, and durability as a published contract

## Status
Accepted

## Context
Three questions, and they are entangled enough that separating them produces
three ADRs that each assume the other two.

1. **Shape.** One `Store` interface, or a layered durable-store-plus-cache?
2. **Durability.** With Redis as the single source of truth, losing the dataset
   breaks every link ever issued, permanently — including ones printed on paper
   (T-15).
3. **Caching versus revocation.** A read cache in front of the store can serve a
   revoked link for the length of its TTL, and can serve it *specifically during
   an outage*, which is when revocation matters most (T-07).

Questions 1 and 3 are the same question. A layered design exists to add a cache;
a cache is what makes revocation racy.

## Decision

### One `Store`, three methods
```go
type Store interface {
    Save(ctx context.Context, l *Link) error
    Load(ctx context.Context, c Code) (*Link, error)
    Revoke(ctx context.Context, c Code, at time.Time) error
}
```
Everything else — owner listing, dedup index, eviction check — is an optional
capability interface that the `Shortener` type-asserts **at construction**, so a
missing capability is a startup error and never a runtime surprise. Three
required methods is three methods every implementer must get right.

### No read cache in v1
This resolves question 3 by removing it. Revocation is immediate because there is
no copy anywhere to invalidate (SR-23). Every resolve is a store round trip, and
ADR-0006 has already accepted that cost.

The alternative — cache with TTL, invalidate on `Revoke` — fails in exactly the
case it would be deployed for. Invalidation is a network operation; during the
partition or outage that makes the cache valuable, invalidation is also the thing
that does not work, and the cache serves the revoked link for the rest of its
TTL. A design whose correctness degrades precisely when it is under load is not a
performance optimization, it is a deferred incident.

### `Save` is conditional, always
`SetNX` semantics. An implementation that overwrites an existing code is a
defect, not a variation, and the contract says so in those words. An
unconditional `SET` silently repoints a distributed link at an attacker's
destination (T-03) — the worst outcome available in this system, reached by the
easiest possible mistake.

Collision returns `ErrCodeExists`, which drives a bounded retry for generated
codes and is returned directly for vanity codes (SR-18, SR-19).

### Fail-closed, not configurable
Store unavailable means `Resolve` returns `ErrStoreUnavailable` and never a
redirect, and `Create` returns an error and never a success. There is no option
to fail open, in the same way `moat`'s rate limiter has none. An option to serve
unvalidated redirects during an outage is a vulnerability with a flag on it.

### Durability is an operational contract, published
Redis is the single source of truth, and the requirements it imposes are part of
the library's contract rather than an assumption left to the operator:

- **AOF with `appendfsync everysec`.** RDB-only snapshots lose every link created
  since the last save.
- **At least one replica**, and a restore procedure that has been exercised. A
  backup nobody has restored is a hope.
- **`maxmemory-policy noeviction`.** Any `allkeys-*` policy silently deletes
  links under memory pressure — a memory spike becomes permanent link loss with
  no error anywhere. `redisstore` implements `EvictionChecker` and
  `Shortener.New` calls it, refusing to start against a misconfigured instance.
  This mirrors `moat`'s check and `usher`'s ADR-3.
- **A dedicated instance or database index**, since cairn's keys are namespaced
  (SR-22) but a shared `FLUSHDB` is not.

`README.md` carries this list, not just this ADR. An operational requirement that
lives only in a decision record is one nobody deploying the library will read.

## Consequences
- Resolution availability equals store availability. Accepted, and stated in the
  README so it is a known property rather than a discovered one.
- A consumer needing a durable non-Redis backend implements `Store` against
  Postgres. The interface is small enough that this is an afternoon, which is the
  main reason it is small.
- No cache means no cache-invalidation bugs, no staleness window to document, and
  no "was it revoked or is it still in the cache" question during an incident.

## Reopening criterion, and what a layered design would have to answer
Reopen when a real deployment measures the store round trip as the redirect
path's bottleneck — a measurement, not an anticipation. Any proposal must answer
all four in writing before code:

1. What is the maximum time a revoked link may still resolve, stated as a number,
   and how is that number defended to a user who revoked a link containing a
   token?
2. What happens to revocation when the cache node cannot be reached — the case
   the cache exists for?
3. Does `Revoke` become a distributed operation that can partially fail, and what
   does it return when it does?
4. What does a cache miss do when the durable store is down — serve stale, or
   fail closed? (SR-20 says fail closed, which removes most of the cache's value
   during an outage, which is worth confronting before building it.)
