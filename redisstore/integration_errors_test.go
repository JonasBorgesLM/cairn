//go:build integration

package redisstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/redisstore"
)

// A dead connection maps to ErrStoreUnavailable on every method, not just
// the one issue #36 exercises through EvictionCheck (NFR-07).
func TestErrorMapping_DeadConnection(t *testing.T) {
	store, err := redisstore.New("127.0.0.1:1")
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	link := mustLink(t, "abc1234567", "https://example.com/")

	if err := store.Save(ctx, link); !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("Save error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
	if _, err := store.Load(ctx, "abc1234567"); !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("Load error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
	if err := store.Revoke(ctx, "abc1234567", link.CreatedAt, false); !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("Revoke error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
}

// redis.Nil must still become ErrCodeNotFound, and only that -- a live
// server answering "no such key" is not the store being unavailable.
func TestErrorMapping_MissAgainstALiveServerIsNotFoundNotUnavailable(t *testing.T) {
	addr, _ := testRedis(t)
	store := newStore(t, addr)

	_, err := store.Load(context.Background(), "doesnotexist")
	if !errors.Is(err, cairn.ErrCodeNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeNotFound)", err)
	}
	if errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("error = %v, must not also be ErrStoreUnavailable", err)
	}
}
