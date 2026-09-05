# ADR-0012: Click counting is a segregated, best-effort interface off the hot path

## Status
Accepted

## Context
Counting resolutions is the most-requested shortener feature and the easiest one
to build badly. Put a counter in `Store` and every resolve becomes a read *and* a
write; a write failure then has to decide whether it fails the redirect. Both
answers are bad: failing the redirect makes an analytics backend a dependency of
link resolution, and swallowing the failure hides it.

## Decision
A separate interface, deliberately not part of `Store`:

```go
type Counter interface {
    Count(ctx context.Context, c Code) error
}
```

Four properties, each of which is the answer to one way this goes wrong:

1. **Segregated.** `Store` implementers are not obliged to write it. A host that
   does not count does not carry the method.
2. **Off the critical path.** `cairnhttp` writes the redirect first, then calls
   `Count`. The redirect is already on the wire when counting begins.
3. **Best-effort.** A `Count` error is reported through `Hooks` and never
   surfaces to the visitor. Analytics is not allowed to break resolution.
4. **Detached context.** `Count` receives `context.WithoutCancel(ctx)` with its
   own timeout — *not* the request context.

Point 4 is not a detail. The request context is cancelled the moment the response
completes, so passing it means every count races the redirect it is counting.
That bug passes tests, because a test that inspects the recorder after the
handler returns has already let the goroutine run, and then drops a variable
fraction of counts under real concurrency. Naming it here is the point of writing
this down.

The default implementation in `redisstore` is a single `INCR` on
`cairn:v1:hits:{code}` with the same TTL as the record, so counts disappear with
the link they belong to.

## Consequences
- Counts are approximate, and the documentation says so rather than implying a
  ledger. A count is lost on process shutdown between response and increment,
  and a messaging app that unfurls a link preview is counted identically to a
  human click — cairn cannot distinguish the two, and neither can anything else
  at this layer.
- No per-click metadata: no timestamps, no referrer, no user agent, no
  geolocation. That is an analytics pipeline, and it is out of scope (§1.2 of the
  requirements). A host that wants one has `Hooks.OnResolve` and `crier`.
- `Counter` returning an error rather than nothing is deliberate: the failure is
  reported to `Hooks`, so "the counter has been broken for a month" is
  discoverable rather than invisible.
- A host counting in a goroutine is subject to shutdown ordering: `cairnhttp`
  provides `Close` so pending counts can be drained on graceful shutdown, and
  loss during a hard kill is accepted and documented.
