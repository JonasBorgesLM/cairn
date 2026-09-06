package cairnhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/memstore"
)

// unavailableStore always reports the store as down, regardless of the code.
type unavailableStore struct{}

func (unavailableStore) Save(context.Context, *cairn.Link) error { return nil }
func (unavailableStore) Load(context.Context, cairn.Code) (*cairn.Link, error) {
	return nil, cairn.ErrStoreUnavailable
}
func (unavailableStore) Revoke(context.Context, cairn.Code, time.Time, bool) error { return nil }

func newUnavailableShortener(t *testing.T) *cairn.Shortener {
	t.Helper()
	s, err := cairn.New(unavailableStore{}, cairn.WithPolicy(allowPolicy{}))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	return s
}

func TestDefaultErrorEncoder_StoreUnavailableIs503NeverARedirect(t *testing.T) {
	h := cairnhttp.NewHandler(newUnavailableShortener(t))
	rec := doGet(h, "/abc1234567")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty (never a redirect for a down store)", got)
	}
}

// A custom encoder that does not itself write a redirect must not end up
// producing one anyway: cairnhttp's own handler code has no fallback path
// that writes Location once an ErrorEncoder has been invoked.
func TestCustomErrorEncoder_StoreUnavailableStillNeverARedirect(t *testing.T) {
	called := false
	h := cairnhttp.NewHandler(newUnavailableShortener(t), cairnhttp.WithErrorEncoder(
		func(w http.ResponseWriter, _ *http.Request, err error) {
			called = true
			// Deliberately minimal: does not classify err at all, just
			// signals a generic failure -- the point is what the *handler*
			// does afterward, not this encoder's own error mapping.
			w.WriteHeader(http.StatusTeapot)
		},
	))

	rec := doGet(h, "/abc1234567")
	if !called {
		t.Fatalf("custom ErrorEncoder was not invoked")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want %d (whatever the custom encoder chose)", rec.Code, http.StatusTeapot)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty", got)
	}
}

// A panicking encoder must not leave a partial redirect behind: cairnhttp
// never writes Location before invoking ErrorEncoder, so there is nothing to
// leak regardless of what the encoder does.
func TestErrorEncoder_PanicLeavesNoPartialRedirect(t *testing.T) {
	h := cairnhttp.NewHandler(newUnavailableShortener(t), cairnhttp.WithErrorEncoder(
		func(http.ResponseWriter, *http.Request, error) {
			panic("boom")
		},
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/abc1234567", http.NoBody)

	func() {
		defer func() { _ = recover() }()
		h.ServeHTTP(rec, req)
	}()

	if rec.Code == http.StatusFound {
		t.Fatalf("status = %d, want anything but 302 after a panic", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty -- a panicking encoder must not leak a partial redirect", got)
	}
}

func TestDefaultErrorEncoder_MapsEachResolveError(t *testing.T) {
	tests := []struct {
		name       string
		shortener  func(t *testing.T) *cairn.Shortener
		code       string
		wantStatus int
	}{
		{"invalid code", newTestShortener, "code:invalid", http.StatusNotFound},
		{"not found", newTestShortener, "abc1234567", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := cairnhttp.NewHandler(tt.shortener(t))
			rec := doGet(h, "/"+tt.code)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestDefaultErrorEncoder_RevokedIs410(t *testing.T) {
	s := newTestShortener(t)
	revoked := createLink(t, s, "https://example.com/revoked")
	if err := s.Revoke(context.Background(), revoked.Code); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	h := cairnhttp.NewHandler(s)
	rec := doGet(h, "/"+string(revoked.Code))
	if rec.Code != http.StatusGone {
		t.Fatalf("revoked link status = %d, want %d", rec.Code, http.StatusGone)
	}
}

func TestDefaultErrorEncoder_ExpiredIs410(t *testing.T) {
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	current := now
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}), cairn.WithClock(func() time.Time { return current }))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	link, err := s.Create(context.Background(), "https://example.com/expiring", cairn.WithTTL(time.Hour))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	current = now.Add(2 * time.Hour)

	h := cairnhttp.NewHandler(s)
	rec := doGet(h, "/"+string(link.Code))
	if rec.Code != http.StatusGone {
		t.Fatalf("expired link status = %d, want %d", rec.Code, http.StatusGone)
	}
}

// ExampleWithErrorEncoder shows the seam task-api uses to keep its own JSON
// error envelope (FR-12, IR-04) instead of cairnhttp's plain-text default.
func ExampleWithErrorEncoder() {
	type envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	taskAPIEnvelope := func(w http.ResponseWriter, _ *http.Request, err error) {
		status, code := http.StatusInternalServerError, "internal_error"
		switch {
		case errors.Is(err, cairn.ErrCodeNotFound), errors.Is(err, cairn.ErrInvalidCode):
			status, code = http.StatusNotFound, "link_not_found"
		case errors.Is(err, cairn.ErrLinkExpired):
			status, code = http.StatusGone, "link_expired"
		case errors.Is(err, cairn.ErrLinkRevoked):
			status, code = http.StatusGone, "link_revoked"
		case errors.Is(err, cairn.ErrStoreUnavailable):
			status, code = http.StatusServiceUnavailable, "temporarily_unavailable"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(envelope{Code: code, Message: err.Error()})
	}

	_ = cairnhttp.WithErrorEncoder(taskAPIEnvelope) // wired into NewHandler(shortener, ...)

	fmt.Println("task-api envelope wired via WithErrorEncoder")

	// Output: task-api envelope wired via WithErrorEncoder
}
