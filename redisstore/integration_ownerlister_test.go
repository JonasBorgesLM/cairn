//go:build integration

package redisstore_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/redisstore"
)

// A record collected by its native TTL leaves no index entry behind: the
// index must not grow past the records it actually points at (issue #45).
//
// Negative control: this test was run against ListByOwner with the ZREM
// cleanup line commented out, and it failed -- ZCARD reported 1, not 0.
// Restored immediately after.
func TestListByOwner_CollectedRecordLeavesNoIndexEntryBehind(t *testing.T) {
	addr, raw := testRedis(t)
	store, err := redisstore.New(addr, redisstore.WithExpiryGrace(0)) // no grace: TTL fires at ExpiresAt itself
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	link := mustLink(t, "abc1234567", "https://example.com/")
	link.OwnerID = "user-1"
	link.ExpiresAt = time.Now().Add(200 * time.Millisecond)
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	// Confirm the record is gone (TTL fired) before asserting on the index.
	deadline := time.Now().Add(5 * time.Second)
	for {
		exists, err := raw.Exists(context.Background(), "cairn:v1:link:abc1234567").Result()
		if err != nil {
			t.Fatalf("EXISTS error = %v", err)
		}
		if exists == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("record was not collected within the deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// ListByOwner must discover the stale entry, not surface it, and clean
	// it up: after this call, the ZSET member is gone.
	links, _, err := store.ListByOwner(context.Background(), "user-1", "", 10)
	if err != nil {
		t.Fatalf("ListByOwner error = %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("ListByOwner returned %d links, want 0 (the only one was collected)", len(links))
	}

	card, err := raw.ZCard(context.Background(), "cairn:v1:owner:user-1").Result()
	if err != nil {
		t.Fatalf("ZCARD error = %v", err)
	}
	if card != 0 {
		t.Fatalf("ZCARD = %d, want 0: the collected record's index entry must not remain", card)
	}
}

// Pagination is stable across concurrent creation against a real Redis, the
// same property proven for memstore: walking every page after N concurrent
// Save calls finish sees exactly N codes, none skipped, none duplicated.
func TestListByOwner_StableAcrossConcurrentCreation(t *testing.T) {
	addr, _ := testRedis(t)
	store := newStore(t, addr)

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := cairn.Code(fmt.Sprintf("owner%07d", i))
			link := mustLink(t, code, fmt.Sprintf("https://example.com/%d", i))
			link.OwnerID = "user-concurrent"
			if err := store.Save(context.Background(), link); err != nil {
				t.Errorf("Save error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	seen := make(map[cairn.Code]bool, n)
	var cursor cairn.Cursor
	for {
		page, next, err := store.ListByOwner(context.Background(), "user-concurrent", cursor, 13)
		if err != nil {
			t.Fatalf("ListByOwner error = %v", err)
		}
		for _, link := range page {
			if seen[link.Code] {
				t.Fatalf("code %q appeared on more than one page", link.Code)
			}
			seen[link.Code] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}

	if len(seen) != n {
		t.Fatalf("saw %d distinct codes across all pages, want %d", len(seen), n)
	}
}
