# ADR-0010: `OwnerID` is first-class data, and never an authorization decision

## Status
Accepted

## Context
`task-api` needs to list a user's links and to let a user revoke their own. That
requires knowing who created a link. Two ways to carry it: an opaque
`Meta map[string]string` that the host fills as it likes, or a typed `OwnerID`
field in `Link`.

Opaque metadata keeps the core smaller and refuses to guess at the host's
identity model. It also pushes every consumer into building the same secondary
index by hand, and each of them gets to invent their own key name and their own
consistency bug between the link record and the index.

## Decision
`Link.OwnerID string` is a first-class field. Empty means unowned, which is a
legitimate state, not an error.

`OwnerLister` is an optional store capability providing cursor-paginated listing
by owner. `redisstore` implements it with a sorted set scored by creation time,
written together with the record in a single Lua script (NFR-11), so the record
and the index cannot diverge under partial failure.

The type is `string` and cairn attaches no meaning to it. It may be a UUID, a
tenant-scoped composite, or an opaque token. cairn compares it and stores it; it
does not parse it.

### The part that matters more than the field
**`Revoke` does not check ownership.** It takes a code and revokes it. It does
not take a caller identity, and it will not grow one.

This is deliberate and it is the reason this ADR exists rather than being a line
in the architecture document. Authorization is the host's, and a library that
guessed at it would be wrong in every deployment whose model differs — an admin
who may revoke anything, a team-shared link, a service account acting for a user,
a support tool. A half-authorization inside the library is worse than none,
because it reads like a guarantee.

The method documentation says this in the imperative: *the caller must have
already authorized this operation; cairn does not check that the caller owns the
link.* It is stated where a reader will meet it, because it is exactly the
assumption a reader makes in the other direction.

`ListByOwner` is not authorization either — it is a filter. Passing an attacker's
chosen `ownerID` returns that owner's links, which is correct behaviour for a
filter and a vulnerability for an unauthenticated endpoint. Also documented at
the method.

## Consequences
- `Link` is not a pure value type any more; it carries an identifier meaningful
  only to the host. Accepted: the alternative made every consumer build the same
  index badly.
- Multi-tenancy is representable by composing the tenant into the `OwnerID`
  (`tenant:user`). cairn does not model tenancy, and a consumer needing tenant
  isolation at the storage layer should use separate keyspaces rather than trust
  string prefixing.
- The owner index grows without bound for a prolific owner. `ListByOwner` is
  cursor-paginated so listing does not, and index entries are removed when the
  record is collected by the same Lua script that writes them.
- `task-api`'s integration is a thin mapping rather than a parallel table, which
  was the point (IR-04).
