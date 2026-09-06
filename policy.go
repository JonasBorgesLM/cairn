package cairn

import "context"

// Decision classifies a destination.
type Decision int

const (
	// Deny rejects the destination at creation.
	Deny Decision = iota

	// Interstitial allows creation but marks the link so a host can warn the
	// visitor before redirecting (FR-13, ADR-0014).
	Interstitial

	// Allow permits the destination outright.
	Allow
)

// Policy classifies a destination. It receives a context because evaluating
// it may resolve names (SR-07).
//
// The zero-value behaviour of a Policy implementation must never be Allow: one
// that cannot reach a resolver, or that hits any other internal failure,
// returns Deny with a reason rather than falling through to permit the
// destination (SR-20).
//
// Policy is declared here, in the core, rather than in package policy, which
// holds the implementations. Policy implementations evaluate a Destination,
// so declaring the interface in policy would make policy import cairn and
// cairn import policy — a cycle. The Go idiom resolves it: the consumer
// declares the interface. One visible consequence is deliberate: New cannot
// default to policy.Default, so it does not default at all — constructing a
// Shortener without a Policy is a configuration error (ADR-0005).
type Policy interface {
	Evaluate(ctx context.Context, d Destination) (Decision, RejectReason, error)
}
