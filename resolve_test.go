package cairn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// The load-bearing ordering of SR-21: alphabet/length validation happens
// before the code ever reaches the store. A counting fake proves it, rather
// than inspecting the implementation.
//
// Negative control: this exact test was run against a build with the order
// reversed (Store.Load called before Alphabet.Validate), and it failed --
// loadCallCount was 1, not 0. The order was restored immediately after.
func TestResolve_InvalidCodePerformsZeroStoreRoundTrips(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	_, err := s.Resolve(context.Background(), cairn.Code("has a space"))
	if !errors.Is(err, cairn.ErrInvalidCode) {
		t.Fatalf("error = %v, want errors.Is(_, ErrInvalidCode)", err)
	}
	if fs.loadCallCount() != 0 {
		t.Fatalf("Load called %d times, want 0 (invalid code must never reach the store)", fs.loadCallCount())
	}
}

func TestResolve_UnknownCodeReturnsErrCodeNotFound(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	_, err := s.Resolve(context.Background(), cairn.Code("abc1234567"))
	if !errors.Is(err, cairn.ErrCodeNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeNotFound)", err)
	}
}

func TestResolve_RevokedLinkReturnsErrLinkRevoked(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	link, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if err = s.Revoke(context.Background(), link.Code); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	_, err = s.Resolve(context.Background(), link.Code)
	if !errors.Is(err, cairn.ErrLinkRevoked) {
		t.Fatalf("error = %v, want errors.Is(_, ErrLinkRevoked)", err)
	}
}

// SR-23: revocation beats caching, by construction -- there is no read cache
// in front of the Store, so a Resolve immediately following a Revoke must see
// it, not a memoized success from the call before.
//
// Negative control: this test was run against a build of Resolve that
// memoized the *Link returned by Store.Load, keyed by code, and served the
// memoized value on a repeat call instead of loading again. It failed -- the
// second Resolve returned the link successfully instead of ErrLinkRevoked.
// The cache was removed immediately after.
func TestResolve_ReflectsRevocationOnTheVeryNextCall(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	link, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	if _, err := s.Resolve(context.Background(), link.Code); err != nil {
		t.Fatalf("first Resolve error = %v, want nil", err)
	}

	if err := s.Revoke(context.Background(), link.Code); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	if _, err := s.Resolve(context.Background(), link.Code); !errors.Is(err, cairn.ErrLinkRevoked) {
		t.Fatalf("second Resolve error = %v, want errors.Is(_, ErrLinkRevoked)", err)
	}
}

func TestResolve_ExpiredLinkReturnsErrLinkExpired(t *testing.T) {
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	fs := newFakeStore()
	current := now
	s := newShortener(t, fs, cairn.WithClock(func() time.Time { return current }))

	link, err := s.Create(context.Background(), "https://example.com/", cairn.WithTTL(time.Hour))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	current = now.Add(2 * time.Hour) // past ExpiresAt
	_, err = s.Resolve(context.Background(), link.Code)
	if !errors.Is(err, cairn.ErrLinkExpired) {
		t.Fatalf("error = %v, want errors.Is(_, ErrLinkExpired)", err)
	}
}

func TestResolve_ActiveLinkSucceedsAndFiresOnResolve(t *testing.T) {
	var got cairn.ResolveEvent
	fired := false
	fs := newFakeStore()
	s, err := cairn.New(fs, cairn.WithPolicy(allowPolicy{}),
		cairn.WithHooks(cairn.Hooks{OnResolve: func(_ context.Context, ev cairn.ResolveEvent) { fired = true; got = ev }}),
	)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	created, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	resolved, err := s.Resolve(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	if resolved.Code != created.Code {
		t.Fatalf("resolved.Code = %q, want %q", resolved.Code, created.Code)
	}
	if !fired {
		t.Fatalf("OnResolve was not fired")
	}
	if got.Code != created.Code {
		t.Fatalf("ResolveEvent.Code = %q, want %q", got.Code, created.Code)
	}
}

func TestResolve_StoreDownReturnsErrStoreUnavailableNeverARedirect(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, &loadFailsStore{fakeStore: fs, err: cairn.ErrStoreUnavailable})

	link, err := s.Resolve(context.Background(), cairn.Code("abc1234567"))
	if !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
	if link != nil {
		t.Fatalf("link = %+v, want nil", link)
	}
}

type loadFailsStore struct {
	*fakeStore
	err error
}

func (s *loadFailsStore) Load(context.Context, cairn.Code) (*cairn.Link, error) {
	return nil, s.err
}
