package cairnhttp

import (
	"context"
	"sync"

	"github.com/JonasBorgesLM/cairn"
)

// counterQueueSize bounds how many pending counts a Handler holds before it
// starts dropping rather than blocking a response. Best-effort, off the
// critical path (FR-14, ADR-0012): a burst large enough to fill this is rare
// enough, and losing a handful of counts is the accepted cost of never
// letting counting slow a redirect.
const counterQueueSize = 256

// runCounter drains h.counterQueue until it is closed, calling Count for
// each code via h.shortener.Count -- which itself runs on a
// context.WithoutCancel derivative, so nothing here needs to worry about a
// request context that is long gone by the time this goroutine gets to a
// given code (ADR-0012).
func (h *Handler) runCounter() {
	defer close(h.counterDone)
	for code := range h.counterQueue {
		if err := h.shortener.Count(context.Background(), code); err != nil {
			h.onCounterError(code, err)
		}
	}
}

// enqueueCount queues code for counting, best-effort: if the queue is full
// or Close has already been called, the count is dropped rather than
// blocking the response that is about to be written. A dropped count is not
// reported anywhere -- WithCounterErrorHandler is for a Counter that ran and
// failed, not for one that never got the chance to run.
func (h *Handler) enqueueCount(code cairn.Code) {
	h.counterMu.Lock()
	defer h.counterMu.Unlock()
	if h.counterClosed {
		return
	}
	select {
	case h.counterQueue <- code:
	default:
	}
}

// Close stops accepting new counts and waits for every count already queued
// to finish, for a graceful shutdown that does not silently drop work still
// in flight. Idempotent: a second call returns nil immediately.
//
// A hard kill (SIGKILL, a crashed process) still loses whatever was queued
// at that instant — there is no journal to replay from. That loss is
// accepted and documented here rather than hidden: Counter is explicitly
// best-effort (FR-14), and the alternative, a durable queue, is a much
// bigger feature this package does not implement.
//
// Call Close after the HTTP server has stopped accepting new requests for
// this Handler (e.g. after http.Server.Shutdown returns) — calling it while
// ServeHTTP may still be running concurrently is safe (it will not panic),
// but any count from a request still in flight at that moment is dropped
// rather than queued.
func (h *Handler) Close() error {
	h.counterMu.Lock()
	if h.counterClosed {
		h.counterMu.Unlock()
		return nil
	}
	h.counterClosed = true
	close(h.counterQueue)
	h.counterMu.Unlock()

	<-h.counterDone
	return nil
}

// counterState is embedded in Handler so its zero value (no NewHandler call
// yet) never panics if ServeHTTP is somehow reached without it -- it always
// goes through NewHandler in practice, which initializes it.
type counterState struct {
	counterMu      sync.Mutex
	counterQueue   chan cairn.Code
	counterDone    chan struct{}
	counterClosed  bool
	onCounterError func(code cairn.Code, err error)
}
