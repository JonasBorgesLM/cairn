# ADR-0003: Format-preserving (Feistel) code generation — considered, deferred

## Status
Accepted — decision is *not in v1*, with a reopening criterion

## Context
A balanced Feistel network keyed with a block cipher gives a **bijection** over
an arbitrary domain — here, the integers `[0, A^L)` rendered in the alphabet. Run
a monotonic counter through it and you get codes that are:

- **unique by construction** — a bijection has no collisions, so `SetNX` never
  fails, no retry loop exists, and `ErrCodeExists` becomes unreachable for
  generated codes;
- **unpredictable without the key** — the output is a pseudorandom permutation,
  so consecutive counter values land in unrelated places;
- **cheap** — a few rounds of HMAC or AES, no round trip, no entropy draw per
  code;
- **dense-space friendly** — the entire domain is usable without the birthday
  penalty that random generation pays.

It is genuinely elegant, and it is the one alternative to ADR-0002 that is not
simply worse. That is exactly why it deserves the analysis in the record rather
than a dismissal.

Note one thing it does *not* buy, because it is easy to assume it does: **scan
resistance is unchanged.** A permutation maps `N` issued counters to `N` points
of the same size space, so density — and therefore `E[hits]` from ADR-0002 — is
identical. Feistel improves collision handling, not enumerability.

## Decision
Not implemented in v1. Three reasons, in increasing order of weight.

**1. It reintroduces the durability problem it was supposed to avoid.** The
counter must be durable, monotonic, and never reused. A counter that resets — a
restored backup, a fresh replica promoted, a misconfigured deploy — re-issues
codes already in use. And unlike random generation, where `SetNX` catches a
collision, here the code path *assumes* uniqueness and has no reason to check.
The failure mode is silently repointing existing links (T-03), which is the
single worst outcome this library has. Keeping `SetNX` as a belt against it
would forfeit the main advantage.

**2. Key rotation is not really available.** The permutation is defined by the
key. Rotate it and previously issued codes decrypt to different counters, so any
scheme that depends on the mapping breaks; keep the key forever and it becomes
an unrotatable, permanently load-bearing secret. Versioning the key with a
prefix rune works but shrinks the alphabet and leaks the rotation epoch into
every code.

**3. Key compromise is a total break, with no analogue in the random design.**
Whoever holds the key can decrypt any observed code to its counter index, learn
exactly how many links the service has ever issued, and then encrypt indices
`0..N` to enumerate **every link that exists** — no scanning, no rate limit to
defeat, no density argument to hide behind. With CSPRNG generation there is no
key, so there is no equivalent event: compromising the process gives an attacker
what that process knows, not the whole corpus.

For a security-first library, trading a retry loop that is cold at the default
density (ADR-0002) for a single secret whose compromise enumerates the entire
dataset is not a good trade.

## Consequences
- Generated codes can collide, so `Save` stays conditional with a bounded retry
  (SR-18, SR-19), and `Hooks.OnRetry` exists to make the retry rate observable
  rather than assumed.
- The `CodeGenerator` interface is shaped so a Feistel generator can be dropped
  in by a consumer who has accepted the three consequences above. The interface
  is not the obstacle; the operational contract is.
- The reasoning is on the record, so a future revisit starts from the argument
  rather than from the idea.

## Reopening criterion
Stated concretely, because "revisit later" without a trigger means never:
reopen if the retry rate reported by `Hooks.OnRetry` exceeds **1% of creations**
sustained over a week in a real deployment, **and** the deployment already has a
durable, monotonic sequence with an operational guarantee against reset. Both
halves, not either. Failing the first, the problem it solves is not a problem;
failing the second, the cure is worse.
