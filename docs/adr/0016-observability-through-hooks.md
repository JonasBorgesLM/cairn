# ADR-0016: Observability is a `Hooks` struct, not an OpenTelemetry dependency

## Status
Accepted

## Context
cairn needs to make creation, resolution, rejection and retry visible. Three
options: import the OpenTelemetry SDK, define an interface per concern
(`Metrics`, `Tracer`, `Logger`), or a struct of optional callbacks.

`crier` already owns log transport in this ecosystem (IR-02), which removes most
of the argument for importing an SDK: cairn would be adding a dependency to
produce data that something else already knows how to ship.

## Decision
A struct of nil-able function fields:

```go
type Hooks struct {
    OnCreate  func(ctx context.Context, ev CreateEvent)
    OnResolve func(ctx context.Context, ev ResolveEvent)
    OnReject  func(ctx context.Context, ev RejectEvent)
    OnRetry   func(ctx context.Context, ev RetryEvent)
}
```

A struct rather than an interface, because an interface forces a consumer who
wants one event to implement four methods, and adding a fifth event later would
be a breaking change to every implementer. A nil field is a no-op; a new field is
additive.

Handlers are **called synchronously and must not block.** cairn holds no
goroutine pool to absorb a slow hook, and `OnResolve` blocking means a redirect
blocking. Stated at the type, in the imperative, because the alternative — cairn
spawning a goroutine per event — trades an obvious problem for an unbounded one.

Event structs carry `Destination` (which redacts itself — SR-15) and typed
`RejectReason` values, never raw URL strings, so a host that logs a whole event
still cannot leak. This is the property that makes hooks safe to hand to an
arbitrary logger, and it is why events do not carry `string` fields for anything
sensitive.

`OnRetry` exists specifically to make ADR-0003's reopening criterion measurable.
Without it, "the retry rate is fine" is an assumption.

## Consequences
- No SDK dependency; NFR-01 and NFR-06 hold.
- Wiring to OpenTelemetry is a consumer-side adapter of a few lines, shown in the
  examples and in `docs/INTEGRATION.md`, rather than an import in the library.
- cairn produces no metrics of its own. A host wanting counters increments them
  in the hooks. Deliberate: a metrics registry is global state, and NFR-04 rules
  it out.
- Span propagation is the host's. Hooks receive the `context.Context` of the
  operation, so a host holding a span can attach to it.
