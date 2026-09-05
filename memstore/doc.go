// Package memstore provides an in-memory cairn.Store, so a consumer can test
// their own integration without a container.
//
// It implements the same conditional-write contract as any other store: a Save
// of an existing code returns cairn.ErrCodeExists and never overwrites (SR-18).
// A memstore that overwrote would let a consumer's tests pass against
// behaviour the real store forbids, which is worse than not shipping one.
package memstore
