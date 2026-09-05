# ADR-0013: Deduplication is opt-in and off by default

## Status
Accepted

## Context
Returning the existing code when a destination has already been shortened saves
storage and gives a stable code per URL. Most shorteners do it.

It is also an existence oracle. With dedup on, any creator can submit a candidate
URL and learn from the response whether *someone else already shortened it*. That
discloses other users' behaviour through the documented API, with no attack
required: submit `https://example.com/internal/q3-results.pdf` and find out
whether anyone on the team has shared it.

There is a second problem that is less obvious. Dedup collapses two links into
one, so revoking "your" link revokes it for everyone who ever shortened that URL,
and the ownership model (ADR-0010) can no longer say who the owner is.

## Decision
Off by default. `WithDeduplication(true)` enables it, and requires a `Store` that
implements `DestIndex`; `New` fails at construction if it does not, rather than
discovering the missing capability on the first create.

When enabled:
- the index keys on `sha256(normalized destination)` — never on the URL itself,
  because key names appear in `SCAN` output, `MONITOR`, and slow-query logs, none
  of which are covered by the redaction of SR-15;
- deduplication is scoped **per owner** when `OwnerID` is set. Two users
  shortening the same URL get two codes; the same user shortening it twice gets
  one. This keeps the storage saving in the case that motivated the feature while
  removing the cross-user oracle entirely, which is the disclosure that actually
  matters;
- the index is written *after* `Save`, so a failure leaves a dangling index entry
  (the next lookup misses, a new link is created, harmless) rather than an index
  entry pointing at a link that was never written;
- a dedup hit returns an existing link whose `ExpiresAt` may differ from what the
  caller requested. The caller's requested TTL is **not** applied to the existing
  link — silently extending a link's life from a create call is an ownership
  violation. The returned link is documented as possibly-not-matching the
  requested options, and a caller who needs their own TTL must disable dedup.

## Consequences
- Off by default means most deployments never meet any of this.
- Per-owner scoping means a global dedup — one code per URL across the whole
  service — is not available. That is intentional: it is the variant with the
  oracle, and a host who insists on it can implement `DestIndex` to ignore the
  owner and thereby own the decision explicitly.
- The oracle within a single owner's own links is not a disclosure: they already
  know what they shortened.
- Normalization (ADR-0015) determines what counts as "the same URL". Because that
  normalization is deliberately minimal, dedup will miss URLs a human considers
  identical. That is the correct direction to err — a false miss creates a
  redundant link; a false hit points a user at someone else's destination.
