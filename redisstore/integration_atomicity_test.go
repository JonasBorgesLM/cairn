//go:build integration

package redisstore_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/JonasBorgesLM/cairn"
)

// The record and its owner-index entry are written by one Lua script so they
// cannot diverge under a partial failure (NFR-11). This drives many
// concurrent Save calls under aggressively short per-call deadlines — short
// enough that a meaningful fraction of round trips are aborted client-side
// mid-flight — and then checks the invariant the script exists to guarantee:
// every link that exists has an owner-index entry, and every owner-index
// entry names a link that exists. A script that wrote the two keys as
// separate commands would let a killed connection land between them and
// break this invariant; a single EVAL cannot, because Redis runs it to
// completion server-side regardless of what happens to the client that sent
// it.
func TestSave_RecordAndOwnerIndexNeverDivergeUnderAbortedConnections(t *testing.T) {
	addr, raw := testRedis(t)
	store := newStore(t, addr)

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// A deadline short enough that many of these round trips are
			// aborted by the client before a reply arrives, without being so
			// short that literally none of them reach Redis at all.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
			defer cancel()
			code := cairn.Code(fmt.Sprintf("code%07d", i))
			_ = store.Save(ctx, mustLink(t, code, fmt.Sprintf("https://example.com/%d", i)))
		}(i)
	}
	wg.Wait()

	assertLinkAndOwnerIndexAgree(t, raw, n)
}

func assertLinkAndOwnerIndexAgree(t *testing.T, raw *redis.Client, n int) {
	t.Helper()
	ctx := context.Background()

	ownerMembers, err := raw.ZRange(ctx, "cairn:v1:owner:", 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRANGE error = %v", err)
	}
	inOwnerIndex := make(map[string]bool, len(ownerMembers))
	for _, m := range ownerMembers {
		inOwnerIndex[m] = true
	}

	for i := range n {
		code := fmt.Sprintf("code%07d", i)
		exists, err := raw.Exists(ctx, "cairn:v1:link:"+code).Result()
		if err != nil {
			t.Fatalf("EXISTS error = %v", err)
		}
		linkExists := exists == 1
		indexed := inOwnerIndex[code]
		if linkExists != indexed {
			t.Fatalf("code %q: link exists=%v, owner-index entry exists=%v -- these must always agree", code, linkExists, indexed)
		}
	}
}
