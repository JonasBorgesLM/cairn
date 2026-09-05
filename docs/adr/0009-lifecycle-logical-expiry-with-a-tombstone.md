# ADR-0009: Logical expiry with a grace window; revocation writes a tombstone

## Status
Accepted

## Context
The easy implementation of expiry is the store's native TTL: set it at write,
let the backend delete the record. It is one line and it is free.

It also makes *expired* and *never existed* indistinguishable. That is genuinely
good for privacy — an enumerating scanner cannot even learn that a code was once
live — and genuinely bad for everything else. A user clicking a link from an old
email is told the link is invalid rather than that it expired. A support
conversation has nothing to look at. An audit trail has a hole exactly where a
question will be asked.

Revocation has the same shape: deleting the record is simplest, and loses the
fact that a deliberate act occurred.

## Decision
**Expiry is a field, not only a TTL.** `Link.ExpiresAt` is written into the
record. The store's TTL is set to `ExpiresAt + grace`, default **30 days**.

- Before `ExpiresAt`: resolves normally.
- Between `ExpiresAt` and `ExpiresAt + grace`: the record is still there, and
  `Resolve` returns `ErrLinkExpired` — distinct from `ErrCodeNotFound` (FR-09).
- After the grace window: collected by the backend, and the two outcomes become
  indistinguishable.

So the privacy end state of the TTL-only design is still reached; it is reached
*deliberately*, after a window in which the system can still answer the question.

**Revocation writes a tombstone.** `Revoke` sets `RevokedAt` and keeps the
record, including the destination. `Resolve` returns `ErrLinkRevoked`. The record
expires on the same grace schedule.

**The expiry check is in cairn, not in the store.** Even where a backend supports
TTL, correctness may not depend on it, because expiry is evaluated against the
configured clock and a store's clock is not cairn's. Native TTL is the garbage
collector; the field is the authority.

## Consequences
- `Resolve` distinguishes three failures. `cairnhttp` maps not-found to 404 and
  expired/revoked to 410 by default — and SR-03 lets a host collapse them to a
  single 404 through the `ErrorEncoder` when it does not want the distinction
  visible to visitors. The library exposes the truth; the transport decides how
  much of it to publish.
- A revoked link's destination stays in the store for the grace window. For a
  user revoking *because* the URL contained a secret, that is the wrong answer.
  `Revoke` therefore takes a `PurgeDestination` variant that tombstones the code
  and clears the destination in the same write: the revocation stays visible, the
  secret does not. The default keeps the destination, because the usual reason to
  revoke is that the link is wrong, not that it is toxic.
- Storage is higher than TTL-only by the grace window's worth of dead records.
  At any plausible link volume this is not a real cost.
- `WithExpiryGrace(0)` gives the pure TTL behaviour for a consumer who wants the
  privacy property immediately. It is available, documented, and not the default.
