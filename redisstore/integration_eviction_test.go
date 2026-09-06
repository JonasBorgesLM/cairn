//go:build integration

package redisstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/redisstore"
)

// Redis's own default maxmemory-policy is noeviction when maxmemory is
// unset, so a fresh container already satisfies ADR-0008 without any
// configuration on this test's part.
func TestEvictionCheck_PassesAgainstTheDefaultPolicy(t *testing.T) {
	addr, _ := testRedis(t)
	store := newStore(t, addr)

	if err := store.EvictionCheck(context.Background()); err != nil {
		t.Fatalf("EvictionCheck error = %v, want nil against the default noeviction policy", err)
	}
}

// An allkeys-* policy silently deletes links under memory pressure. New
// refuses to start against one (this test exercises EvictionCheck directly;
// M4's Shortener.New calling it is tested there against a fake).
func TestEvictionCheck_FailsAgainstAnEvictingPolicy(t *testing.T) {
	addr, raw := testRedis(t)
	if err := raw.ConfigSet(context.Background(), "maxmemory-policy", "allkeys-lru").Err(); err != nil {
		t.Fatalf("CONFIG SET error = %v", err)
	}
	store := newStore(t, addr)

	err := store.EvictionCheck(context.Background())
	if err == nil {
		t.Fatalf("EvictionCheck error = nil, want non-nil against allkeys-lru")
	}
	if !strings.Contains(err.Error(), "ADR-0008") {
		t.Fatalf("error = %q, want it to name ADR-0008", err.Error())
	}
	if !strings.Contains(err.Error(), "noeviction") {
		t.Fatalf("error = %q, want it to name the setting to change", err.Error())
	}
}

// An unreachable Redis at startup fails closed, not open: EvictionCheck must
// return an error, never a silent pass, when it cannot even connect.
func TestEvictionCheck_UnreachableRedisFailsClosed(t *testing.T) {
	// Port 1 is privileged and nothing listens on it; the connection attempt
	// fails fast rather than hanging on a routable-but-silent address.
	store, err := redisstore.New("127.0.0.1:1")
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	err = store.EvictionCheck(context.Background())
	if err == nil {
		t.Fatalf("EvictionCheck error = nil, want non-nil against an unreachable Redis")
	}
	if !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
}
