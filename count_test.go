package cairn_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

type recordingCounter struct {
	mu     sync.Mutex
	counts []cairn.Code
}

func (c *recordingCounter) Count(_ context.Context, code cairn.Code) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts = append(c.counts, code)
	return nil
}

func (c *recordingCounter) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.counts)
}

func TestShortener_Count_NoCounterConfiguredIsANoOp(t *testing.T) {
	s := newShortener(t, newFakeStore())
	if err := s.Count(context.Background(), "abc1234567"); err != nil {
		t.Fatalf("Count error = %v, want nil (no Counter configured)", err)
	}
}

func TestShortener_Count_ForwardsToTheConfiguredCounter(t *testing.T) {
	counter := &recordingCounter{}
	s := newShortener(t, newFakeStore(), cairn.WithCounter(counter))

	if err := s.Count(context.Background(), "abc1234567"); err != nil {
		t.Fatalf("Count error = %v", err)
	}
	if counter.len() != 1 {
		t.Fatalf("counter recorded %d counts, want 1", counter.len())
	}
}

// This is the bug ADR-0012 exists to prevent: using the request context
// directly. It is cancelled the moment the response completes, so a count
// issued right after would be aborted before it reaches the Counter. Count
// must run on a context.WithoutCancel derivative so an already-cancelled
// caller context does not stop the count from being recorded.
//
// Negative control: this test was run against a build of Count that passed
// ctx straight through instead of context.WithoutCancel(ctx), and it failed
// -- the recording counter saw zero calls, because the fake Counter checked
// ctx.Err() itself (as a real one plausibly would, e.g. before a network
// call) and refused to run against an already-cancelled context. Restored
// immediately after.
func TestShortener_Count_SucceedsWithAnAlreadyCancelledContext(t *testing.T) {
	counter := &cancelCheckingCounter{}
	s := newShortener(t, newFakeStore(), cairn.WithCounter(counter))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Count is even called

	if err := s.Count(ctx, "abc1234567"); err != nil {
		t.Fatalf("Count error = %v, want nil even with an already-cancelled ctx", err)
	}
	if counter.calls != 1 {
		t.Fatalf("counter was called %d times, want 1 -- an already-cancelled request context must not stop the count", counter.calls)
	}
}

// cancelCheckingCounter refuses to run against an already-cancelled
// context, the way a real network-backed Counter plausibly would.
type cancelCheckingCounter struct{ calls int }

func (c *cancelCheckingCounter) Count(ctx context.Context, _ cairn.Code) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.calls++
	return nil
}

func TestShortener_Count_ErrorPropagatesToTheCaller(t *testing.T) {
	boom := errors.New("boom")
	counter := &erroringCounter{err: boom}
	s := newShortener(t, newFakeStore(), cairn.WithCounter(counter))

	err := s.Count(context.Background(), "abc1234567")
	if !errors.Is(err, boom) {
		t.Fatalf("Count error = %v, want errors.Is(_, boom)", err)
	}
}

type erroringCounter struct{ err error }

func (c *erroringCounter) Count(context.Context, cairn.Code) error { return c.err }
