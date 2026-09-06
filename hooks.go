package cairn

import "context"

// Hooks receive lifecycle events. Every field may be nil; nil is a no-op.
//
// Handlers are called synchronously and must not block: cairn holds no
// goroutine pool to absorb a slow hook, and a hook that blocks Resolve blocks
// a redirect. Route them into a proper observability pipeline — asynchronously,
// from the host — rather than doing slow work here (IR-02, ADR-0016).
type Hooks struct {
	// OnCreate fires after a link is successfully created.
	OnCreate func(ctx context.Context, ev CreateEvent)

	// OnResolve fires after a code is successfully resolved.
	OnResolve func(ctx context.Context, ev ResolveEvent)

	// OnReject fires when a destination is rejected, whether by
	// ParseDestination's syntax checks or by a Policy.
	OnReject func(ctx context.Context, ev RejectEvent)

	// OnRetry fires on each retried attempt of a colliding code generation
	// (SR-19).
	OnRetry func(ctx context.Context, ev RetryEvent)
}

// CreateEvent describes a successful Create. Dest is a [Destination], which
// redacts itself through every formatting and logging path, so a handler that
// logs the whole event still cannot leak the URL (SR-15).
type CreateEvent struct {
	Code    Code
	Dest    Destination
	OwnerID string
}

// ResolveEvent describes a successful Resolve.
type ResolveEvent struct {
	Code Code
	Dest Destination
}

// RejectEvent describes a rejected destination. Reason is a typed
// [RejectReason], never a raw message, so a handler can branch on it without
// matching on a string.
type RejectEvent struct {
	Dest   Destination
	Reason RejectReason
}

// RetryEvent describes one retried attempt after a code collision.
type RetryEvent struct {
	Code    Code
	Attempt int
}
