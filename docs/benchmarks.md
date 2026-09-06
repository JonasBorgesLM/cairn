# Benchmarks

Issue #50. `go test -bench` output, Apple M1, `go1.27.1`, recorded 2026-09-06.
Re-run with `GOWORK=off go test . -bench=. -benchmem -run '^$'` (core) and
`GOWORK=off go test -tags=integration -bench=. -benchmem -run '^$' ./...`
(redisstore, needs Docker) to reproduce; absolute numbers will vary by
machine, the percentages in §3 are the part that matters.

## 1. Code generation and validation

```
BenchmarkAlphabetBase62_Validate-8       21978229    53.71 ns/op      0 B/op   0 allocs/op
BenchmarkNewRandomGenerator_Generate-8    3698072   324.5 ns/op      96 B/op   3 allocs/op
```

`Validate` (SR-21's before-the-key gate) is allocation-free. `Generate`
(rejection sampling over `crypto/rand`, ADR-0002) costs three allocations —
the `crypto/rand` read buffer, the rune buffer, and the `Code` string
conversion — and is not on the resolve path at all, only on create.

## 2. Full resolve path

```
BenchmarkShortener_Resolve_Memstore-8    9515492    126.6 ns/op    176 B/op   1 allocs/op
```

memstore's `Resolve` (alphabet validation, map lookup, revoked/expired
checks) is the in-process floor: no network, no serialization. It is not
"realistic redirect-path latency" — §3 uses the real-Redis numbers for that —
but it is the number a consumer embedding cairn with an in-memory `Store`
actually gets.

Against real Redis (`redisstore`, NFR-10, via testcontainers), `Resolve`
including the network round trip:

```
BenchmarkShortener_Resolve_Redis/32bytes-8      9204    129599 ns/op   1881 B/op   36 allocs/op
BenchmarkShortener_Resolve_Redis/128bytes-8     9099    125193 ns/op   2088 B/op   36 allocs/op
BenchmarkShortener_Resolve_Redis/512bytes-8    10000    127661 ns/op   2904 B/op   36 allocs/op
BenchmarkShortener_Resolve_Redis/2000bytes-8    8312    144693 ns/op   5912 B/op   36 allocs/op
```

Flat at ~125-145μs regardless of destination length: dominated by the
localhost TCP round trip and Lua script execution in Redis (`save.lua` is
not on this path, but `HGetAll` plus the client's own overhead is), not by
payload size.

## 3. The unmask (ADR-0007 objection 3)

`Destination.URL()` runs the HMAC-SHA-256 keystream derivation `moat/secret`
uses to unmask a `secret.Value`. Every redirect calls it once, to build the
`Location` header:

```
BenchmarkDestination_URL/32bytes-8      2070520     586.4 ns/op    976 B/op   11 allocs/op
BenchmarkDestination_URL/128bytes-8     1000000    1200   ns/op   1168 B/op   11 allocs/op
BenchmarkDestination_URL/512bytes-8      339944    3546   ns/op   1936 B/op   11 allocs/op
BenchmarkDestination_URL/2000bytes-8      94855   12611   ns/op   5008 B/op   11 allocs/op
```

Unlike the store round trip, this scales with URL length — the keystream
covers every byte of the raw destination.

### The percentage ADR-0007's reopening criterion asks for

*"the benchmark shows the unmask exceeding 1% of redirect-path latency at
realistic URL lengths"*

"Redirect-path latency" is `Resolve` against a real store plus the unmask —
the cairnhttp core module cannot benchmark `cairnhttp` against `redisstore`
directly (ADR-0001: the core does not import the satellite module), so this
composes the two measurements above rather than measuring one call:

| Destination length | Resolve (real Redis) | Unmask | Unmask as % of (Resolve + unmask) |
| --- | --- | --- | --- |
| 32 bytes | 129,599 ns | 586 ns | **0.45%** |
| 128 bytes | 125,193 ns | 1,200 ns | **0.95%** |
| 512 bytes | 127,661 ns | 3,546 ns | **2.70%** |
| 2,000 bytes | 144,693 ns | 12,611 ns | **8.02%** |

At 32 and 128 bytes — the length of an ordinary URL, which is what most
shortened links are — the unmask stays under the 1% line. At 512 bytes and
above — a pre-signed S3 URL or a token-carrying redirect, exactly the
payloads ADR-0007's own `Context` section names as the reason `Destination`
needs redaction in the first place — **it exceeds 1%**, reaching 8% at the
2000-byte ceiling SR-08 allows.

Read literally, this meets ADR-0007's stated reopening criterion at longer,
and not unrealistic, URL lengths. This document reports the measurement;
it does not reopen the ADR — that is a decision for whoever owns the
trade-off ADR-0007 recorded (drop the `moat` dependency vs. accept the cost
at long URLs), not something a benchmark run decides by itself. See issue
#74.

For context, the same unmask against memstore's near-zero store cost (§2)
is a much larger fraction of the *in-process* total (`BenchmarkHandler_ServeHTTP`,
25%-76% across the same lengths) — which is why this document uses the
real-Redis denominator rather than memstore's: an artificially fast store
makes any fixed-cost step look disproportionately expensive, and ADR-0007's
criterion is about production latency, not an in-memory test double's.

## 4. Reproducing

```bash
# Core module: generation, validation, unmask, memstore resolve.
GOWORK=off go test . -bench=. -benchmem -run '^$'
GOWORK=off go test ./cairnhttp/... -bench=. -benchmem -run '^$'

# redisstore: full resolve path against real Redis (needs Docker).
cd redisstore && GOWORK=off go test -tags=integration -bench=. -benchmem -run '^$' ./...
```
