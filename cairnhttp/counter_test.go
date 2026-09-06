package cairnhttp_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/memstore"
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

func (c *recordingCounter) codes() []cairn.Code {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cairn.Code(nil), c.counts...)
}

func newCountingShortener(t *testing.T, counter cairn.Counter) *cairn.Shortener {
	t.Helper()
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}), cairn.WithCounter(counter))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	return s
}

// A successful redirect eventually counts. Close() drains every queued
// count before returning, which is what makes this deterministic without a
// sleep or a poll loop.
func TestHandler_RedirectCounts(t *testing.T) {
	counter := &recordingCounter{}
	s := newCountingShortener(t, counter)
	link := createLink(t, s, "https://example.com/docs")

	h := cairnhttp.NewHandler(s)
	rec := doGet(h, "/"+string(link.Code))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	codes := counter.codes()
	if len(codes) != 1 || codes[0] != link.Code {
		t.Fatalf("counter recorded %v, want [%q]", codes, link.Code)
	}
}

// A resolve failure never counts: Counter records a resolution, not an
// attempt.
func TestHandler_ErrorPathDoesNotCount(t *testing.T) {
	counter := &recordingCounter{}
	s := newCountingShortener(t, counter)

	h := cairnhttp.NewHandler(s)
	doGet(h, "/doesnotexist1")

	if err := h.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if len(counter.codes()) != 0 {
		t.Fatalf("counter recorded %v, want none", counter.codes())
	}
}

// Showing the interstitial warning page is not itself a completed
// resolution -- only an actual redirect (direct, or via ?continue=1) counts.
func TestHandler_InterstitialPageDoesNotCount(t *testing.T) {
	counter := &recordingCounter{}
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(interstitialPolicy{}), cairn.WithCounter(counter))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	link := createLink(t, s, "https://unknown.example/")

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	doGet(h, "/"+string(link.Code)) // no ?continue=1: shows the warning page

	if err := h.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if len(counter.codes()) != 0 {
		t.Fatalf("counter recorded %v for the warning page, want none", counter.codes())
	}

	rec := doGet(h, "/"+string(link.Code)+"?continue=1")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
}

// A Counter error must never change the response: counting happens after
// the response is already written, off the critical path (ADR-0012, FR-14).
func TestHandler_CountErrorDoesNotChangeTheResponse(t *testing.T) {
	s := newCountingShortener(t, alwaysErrorsCounter{})
	link := createLink(t, s, "https://example.com/docs")

	var reportedErr error
	var reportedCode cairn.Code
	h := cairnhttp.NewHandler(s, cairnhttp.WithCounterErrorHandler(func(code cairn.Code, err error) {
		reportedCode, reportedErr = code, err
	}))

	rec := doGet(h, "/"+string(link.Code))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (unaffected by the counter failing)", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "https://example.com/docs" {
		t.Fatalf("Location = %q, unaffected by the counter failing", got)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if reportedErr == nil {
		t.Fatalf("WithCounterErrorHandler was not called")
	}
	if reportedCode != link.Code {
		t.Fatalf("reported code = %q, want %q", reportedCode, link.Code)
	}
}

type alwaysErrorsCounter struct{}

func (alwaysErrorsCounter) Count(context.Context, cairn.Code) error {
	return errors.New("boom")
}

// Close is idempotent and drains whatever was queued before it was called.
func TestHandler_Close_DrainsQueuedCountsAndIsIdempotent(t *testing.T) {
	counter := &recordingCounter{}
	s := newCountingShortener(t, counter)
	link := createLink(t, s, "https://example.com/docs")

	h := cairnhttp.NewHandler(s)
	for range 10 {
		doGet(h, "/"+string(link.Code))
	}

	if err := h.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("second Close error = %v, want nil (idempotent)", err)
	}

	if len(counter.codes()) != 10 {
		t.Fatalf("counter recorded %d counts, want 10 (all drained)", len(counter.codes()))
	}
}
