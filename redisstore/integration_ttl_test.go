//go:build integration

package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn/redisstore"
)

// ADR-0009: the store's native TTL is set to ExpiresAt + grace, never to
// ExpiresAt alone — the grace window is what lets a store round trip during
// it still answer ErrLinkExpired instead of ErrCodeNotFound.
func TestSave_NativeTTLIsExpiresAtPlusGrace(t *testing.T) {
	addr, raw := testRedis(t)
	const grace = 5 * time.Second
	store, err := redisstore.New(addr, redisstore.WithExpiryGrace(grace))
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now()
	link := mustLink(t, "abc1234567", "https://example.com/")
	link.CreatedAt = now
	link.ExpiresAt = now.Add(3 * time.Second)

	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	ttl, err := raw.TTL(context.Background(), "cairn:v1:link:abc1234567").Result()
	if err != nil {
		t.Fatalf("TTL error = %v", err)
	}

	want := 3*time.Second + grace
	// Wall-clock slack for the round trip itself; the property under test is
	// "roughly ExpiresAt+grace", not an exact tick.
	const slack = 2 * time.Second
	if ttl < want-slack || ttl > want {
		t.Fatalf("TTL = %v, want close to %v (ExpiresAt-now + grace)", ttl, want)
	}
}

// A link with no logical expiry gets no native TTL: it must not be collected
// by Redis on its own initiative (ExpiresAt, not the store's clock, is the
// authority — ADR-0009).
func TestSave_NoExpiresAtSetsNoTTL(t *testing.T) {
	addr, raw := testRedis(t)
	store := newStore(t, addr)

	link := mustLink(t, "abc1234567", "https://example.com/")
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	ttl, err := raw.TTL(context.Background(), "cairn:v1:link:abc1234567").Result()
	if err != nil {
		t.Fatalf("TTL error = %v", err)
	}
	if ttl != -1 {
		t.Fatalf("TTL = %v, want -1 (no expiry) for a link with a zero ExpiresAt", ttl)
	}
}

// Between ExpiresAt and ExpiresAt+grace, the record is still in the store:
// the logical-expiry check is cairn's, not Redis's (ADR-0009). This proves
// the store side of that contract — that the record survives past
// ExpiresAt — not the Shortener's IsExpired check, which is tested at M4.
func TestLoad_RecordSurvivesPastExpiresAtWithinTheGraceWindow(t *testing.T) {
	addr, _ := testRedis(t)
	store, err := redisstore.New(addr, redisstore.WithExpiryGrace(10*time.Second))
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	link := mustLink(t, "abc1234567", "https://example.com/")
	link.ExpiresAt = time.Now().Add(500 * time.Millisecond)
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	time.Sleep(700 * time.Millisecond) // past ExpiresAt, well within the 10s grace

	got, err := store.Load(context.Background(), "abc1234567")
	if err != nil {
		t.Fatalf("Load error = %v, want the record still present during the grace window", err)
	}
	if !got.ExpiresAt.Equal(link.ExpiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, link.ExpiresAt)
	}
}
