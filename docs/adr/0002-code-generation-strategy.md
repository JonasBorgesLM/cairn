# ADR-0002: Code generation is CSPRNG-drawn, and length is a security parameter

## Status
Accepted

## Context
The default design for a shortener is an auto-increment counter rendered in
base62. It is compact, collision-free and free of round trips. It is also
enumerable: given one code, an attacker walks the entire history of the service
backwards and forwards. This is discarded (SR-01).

That much is easy. The part that is usually got wrong is treating "we use random
codes" as the end of the analysis, when there are **two independent properties**
here and randomness only supplies one of them.

**Property 1 — unpredictability.** Given any set of issued codes, the next one
cannot be predicted better than chance. This is a property of the *source*. It is
supplied by `crypto/rand` and destroyed by a counter, a timestamp, a
`math/rand` generator, or a hash of the destination — the last being the
subtlest, because a hash looks random while being a deterministic function of a
guessable input.

**Property 2 — scan resistance.** The space cannot be swept to discover links
belonging to other people. This is a property of the *density*, not of the
source. A perfectly random 4-character code is unpredictable and trivially
scannable. For `N` live links, alphabet size `A`, length `L`, and an attacker
issuing `R` requests:

```
E[hits] = R · N / A^L
```

Randomness contributes nothing to this expression. Length does.

The 2016 enumeration of a major provider's 6-character codes is the worked
example: those codes were not sequential. Density was the missing property, and
the result was a public corpus of documents and mapped routes.

## Decision
Default generator: rejection sampling over `crypto/rand` (SR-01). Rejection
sampling and not modulo reduction — `b % 62` over a uniform byte biases the
first 8 runes of the alphabet upward, which shrinks the effective keyspace by a
fraction of a bit for free and for no reason.

Default length: **10 runes over base62**, giving `62^10 ≈ 8.4 × 10^17` and
≈59.5 bits.

Worked, so the number is inspectable rather than asserted — one million live
links, an attacker sustaining 10,000 requests per second for thirty days
(2.6 × 10^10 requests):

| `L` | Keyspace | E[hits] |
| --- | --- | --- |
| 7 | 3.5 × 10^12 | ~7,400 |
| 8 | 2.2 × 10^14 | ~119 |
| 10 | 8.4 × 10^17 | ~0.03 |
| 12 | 3.2 × 10^21 | ~8 × 10^-6 |

Seven characters — a common choice — hands a scanner thousands of other people's
links. Ten costs three characters and moves it to a thirty-year event.

Rate limiting reduces `R`, and the host supplies it through `moat` (IR-01). It is
**not** counted as a mitigation for scan resistance, because a distributed
scanner defeats per-IP limiting and density has to hold without it.

`New` takes `WithExpectedLinks(n)` and **refuses a configuration whose
`E[hits]` is self-evidently unsafe at the declared `n`** (NFR-08). A consumer who
sets `WithCodeLength(5)` for prettier links gets a construction error with the
arithmetic in the message, not a runtime warning nobody reads.

## Consequences
- Codes are ten characters. They are longer than a competitor's. The trade is
  stated in the README next to the number, because a consumer who does not
  understand why will shorten it.
- Every `Save` is a conditional write with a possible retry, since random
  generation can collide (SR-18, SR-19). At the default density the retry path
  is effectively cold; `Hooks.OnRetry` exists so a host can find out if that
  stops being true.
- `CodeGenerator` is an interface, so a consumer can reintroduce every problem
  above. Its documentation says so in the strongest terms the godoc will carry.
- The density formula is published in the package documentation, so a host with
  10^10 links can compute their own `L` instead of trusting a default sized for
  someone else.

## Related
ADR-0003 records the format-preserving alternative that was considered and not
taken. ADR-0004 records what the alphabet choice costs in this arithmetic.
