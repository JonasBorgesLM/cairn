package memstore_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/memstore"
	"github.com/JonasBorgesLM/cairn/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func() cairn.Store { return memstore.New() })
}

// Concurrent Save of the same code: exactly one must win. This is SR-18's
// property under the condition that actually exercises it -- a single
// winner is trivial without concurrency.
func TestSave_ConcurrentSameCodeExactlyOneWins(t *testing.T) {
	s := memstore.New()
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.Save(context.Background(), &cairn.Link{Code: "abc1234567", Dest: dest})
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

func TestOwnerLister_ListsInCreationOrderAndPaginates(t *testing.T) {
	s := memstore.New()
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	codes := []cairn.Code{"code0000001", "code0000002", "code0000003"}
	for _, c := range codes {
		if err = s.Save(context.Background(), &cairn.Link{Code: c, Dest: dest, OwnerID: "user-1"}); err != nil {
			t.Fatalf("Save error = %v", err)
		}
	}

	page1, cursor, err := s.ListByOwner(context.Background(), "user-1", "", 2)
	if err != nil {
		t.Fatalf("ListByOwner error = %v", err)
	}
	if len(page1) != 2 || page1[0].Code != codes[0] || page1[1].Code != codes[1] {
		t.Fatalf("page1 = %v, want first two codes in order", page1)
	}
	if cursor == "" {
		t.Fatalf("cursor = empty, want a continuation token (more results remain)")
	}

	page2, cursor2, err := s.ListByOwner(context.Background(), "user-1", cursor, 2)
	if err != nil {
		t.Fatalf("ListByOwner error = %v", err)
	}
	if len(page2) != 1 || page2[0].Code != codes[2] {
		t.Fatalf("page2 = %v, want the last code", page2)
	}
	if cursor2 != "" {
		t.Fatalf("cursor2 = %q, want empty (no more results)", cursor2)
	}
}

func TestOwnerLister_UnknownOwnerReturnsEmpty(t *testing.T) {
	s := memstore.New()
	links, cursor, err := s.ListByOwner(context.Background(), "nobody", "", 10)
	if err != nil {
		t.Fatalf("ListByOwner error = %v", err)
	}
	if len(links) != 0 || cursor != "" {
		t.Fatalf("links = %v, cursor = %q, want empty", links, cursor)
	}
}

func TestDestIndex_RoundTripsAndScopesPerOwner(t *testing.T) {
	s := memstore.New()
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}

	if err = s.IndexDest(context.Background(), "user-1", dest, "abc1234567"); err != nil {
		t.Fatalf("IndexDest error = %v", err)
	}

	code, err := s.LookupByDest(context.Background(), "user-1", dest)
	if err != nil {
		t.Fatalf("LookupByDest error = %v", err)
	}
	if code != "abc1234567" {
		t.Fatalf("code = %q, want %q", code, "abc1234567")
	}

	_, err = s.LookupByDest(context.Background(), "user-2", dest)
	if !errors.Is(err, cairn.ErrCodeNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeNotFound) (index is per-owner)", err)
	}
}

func TestCounter_CountsResolutions(t *testing.T) {
	s := memstore.New()
	for range 3 {
		if err := s.Count(context.Background(), "abc1234567"); err != nil {
			t.Fatalf("Count error = %v", err)
		}
	}
	if got := s.CountOf("abc1234567"); got != 3 {
		t.Fatalf("CountOf = %d, want 3", got)
	}
}

var (
	_ cairn.Store       = (*memstore.Store)(nil)
	_ cairn.OwnerLister = (*memstore.Store)(nil)
	_ cairn.DestIndex   = (*memstore.Store)(nil)
	_ cairn.Counter     = (*memstore.Store)(nil)
)
