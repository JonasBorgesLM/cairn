// Package policy provides destination policy implementations for cairn.
//
// The Policy interface and the Decision type are declared in the parent
// package, not here. That is deliberate: a policy evaluates a
// cairn.Destination, so declaring the interface here would be an import cycle.
// The Go idiom resolves it by having the consumer declare the interface, and
// it has a welcome consequence — cairn.New cannot default to a policy from
// this package, so it does not default at all, and a shortener constructed
// without one is a configuration error rather than a quiet weakening.
//
// See docs/adr/0005-destination-policy-and-the-ssrf-boundary.md.
package policy
