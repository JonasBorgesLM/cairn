//go:build integration

package redisstore_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// A second Save of the same code returns ErrCodeExists and leaves the
// original record byte-identical — not merely "still present", but
// unchanged down to the field an attacker's Save tried to plant (SR-18).
//
// Negative control: this exact test was run against a save.lua temporarily
// edited to skip the EXISTS check (an unconditional HSET, the SET-shaped
// bug SR-18 exists to rule out), and it failed: the record held the
// attacker's destination, not the original. The script was restored
// immediately after; see the commit for the observed failure.
func TestSave_SecondSaveLeavesOriginalByteIdentical(t *testing.T) {
	addr, _ := testRedis(t)
	store := newStore(t, addr)
	ctx := context.Background()

	original := mustLink(t, "abc1234567", "https://example.com/original")
	original.CreatedAt = time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Save(ctx, original); err != nil {
		t.Fatalf("first Save error = %v", err)
	}

	attacker := mustLink(t, "abc1234567", "https://evil.example/")
	attacker.CreatedAt = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	err := store.Save(ctx, attacker)
	if !errors.Is(err, cairn.ErrCodeExists) {
		t.Fatalf("second Save error = %v, want errors.Is(_, ErrCodeExists)", err)
	}

	got, err := store.Load(ctx, "abc1234567")
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if !got.Dest.Equal(original.Dest) {
		t.Fatalf("record was overwritten by the second Save")
	}
	if !got.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("CreatedAt changed: got %v, want %v (record must be byte-identical)", got.CreatedAt, original.CreatedAt)
	}
}

// Concurrent creation of the same code from many goroutines yields exactly
// one winner — the property that only holds under real concurrency against
// a real server, which is the whole reason NFR-10 forbids miniredis here.
func TestSave_ConcurrentSameCodeExactlyOneWinner(t *testing.T) {
	addr, _ := testRedis(t)
	store := newStore(t, addr)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = store.Save(ctx, mustLink(t, "concurrent1", fmt.Sprintf("https://example.com/%d", i)))
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, cairn.ErrCodeExists) {
			t.Fatalf("unexpected Save error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winning Save calls = %d, want exactly 1", wins)
	}
}

func mustLink(t *testing.T, code cairn.Code, rawURL string) *cairn.Link {
	t.Helper()
	dest, err := cairn.ParseDestination(rawURL)
	if err != nil {
		t.Fatalf("ParseDestination(%q) error = %v", rawURL, err)
	}
	return &cairn.Link{Code: code, Dest: dest}
}
