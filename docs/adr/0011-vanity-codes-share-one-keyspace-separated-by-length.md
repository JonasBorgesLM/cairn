# ADR-0011: Vanity codes share the generated keyspace and are separated by length

## Status
Accepted

## Context
Caller-chosen codes (`/launch`, `/q3-report`) are a normal product requirement.
They raise three problems:

1. **Collision with the generated space.** If a vanity code can be a string the
   generator might also produce, two links can claim one code, and whichever
   arrives second either fails confusingly or — much worse — overwrites.
2. **Squatting.** An attacker registers vanity codes that the generator will want
   later, degrading the generation path into a retry loop.
3. **Availability probing.** Asking whether a vanity code is free is an oracle,
   and in the worst design it is an oracle over the *generated* space: submit
   candidate generated-length codes and learn which are taken, without ever
   issuing a request to the resolve endpoint where rate limiting lives.

An early sketch of this design put vanity codes behind a separate key prefix.
That is wrong and worth recording as wrong: resolution receives only a code, with
nothing to say which namespace it belongs to, so two prefixes mean either two
lookups per resolve or an ambiguous mapping. Neither is acceptable on the read
path.

## Decision
**One keyspace.** Generated and vanity codes are stored under the same key
pattern, so resolution stays a single lookup and `SetNX` remains the collision
authority (SR-18).

**Separation by length.** The generated length is fixed by configuration
(default 10). A vanity code of *exactly* that length is rejected with
`ErrVanityLength`. Vanity codes live in `[minLen, maxLen]` with the generated
length excised from the range.

This one rule solves all three problems at once:

- Collision (1) becomes structurally impossible rather than caught late.
- Squatting (2) becomes impossible: no vanity code can occupy any string the
  generator can produce.
- Probing the generated space (3) becomes impossible: submitting a
  generated-length string is rejected on length, before any store lookup, so it
  returns nothing about occupancy (SR-04).

**Reserved words.** A default list — `admin`, `api`, `health`, `login`,
`static`, `assets`, `robots.txt`, `favicon.ico`, `.well-known`, and similar —
rejected with `ErrVanityReserved`, extendable via `WithVanity`. Without it a
vanity code shadows the host's own routes, which is a routing bug that presents
as a security incident.

**No retry for vanity codes.** `ErrCodeExists` is returned to the caller.
Retrying would silently issue a different code than the one they asked for.

## Consequences
- Vanity availability within the *vanity* range remains an oracle for an
  authenticated creator (T-09, SR-16). This is inherent — DNS has the same
  property — and is documented rather than papered over. The mitigations are the
  host's: authentication and `moat` rate limiting on the create route.
- Changing the generated length on a live deployment can make a previously legal
  vanity code collide with the new generated length. `New` cannot detect this,
  so the constraint is documented as a migration hazard: changing
  `WithCodeLength` requires auditing existing vanity codes.
- Vanity codes are short and therefore individually guessable. That is what the
  user asked for by choosing a memorable code, and the documentation states that
  a vanity code is a *public* name, never a capability. A link that must be
  unguessable must not be vanity.
- `Link.Vanity` is stored, so a host can filter or apply different policy to
  vanity links without inferring it from length.
