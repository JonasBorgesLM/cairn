package cairn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

func newShortener(t *testing.T, store cairn.Store, opts ...cairn.Option) *cairn.Shortener {
	t.Helper()
	s, err := cairn.New(store, append([]cairn.Option{cairn.WithPolicy(allowPolicy{})}, opts...)...)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	return s
}

func TestCreate_RejectsAnInvalidDestination(t *testing.T) {
	s := newShortener(t, newFakeStore())
	_, err := s.Create(context.Background(), "javascript:alert(1)")
	if err == nil {
		t.Fatalf("Create error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonScheme {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonScheme)
	}
}

func TestCreate_DeniedByPolicyFiresOnRejectAndReturnsNoLink(t *testing.T) {
	var rejected cairn.RejectEvent
	fired := false
	hooks := cairn.Hooks{OnReject: func(_ context.Context, ev cairn.RejectEvent) { fired = true; rejected = ev }}

	fs := newFakeStore()
	s, err := cairn.New(fs, cairn.WithPolicy(stubPolicy{decision: cairn.Deny, reason: cairn.ReasonOwnDomain}), cairn.WithHooks(hooks))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	link, err := s.Create(context.Background(), "https://example.com/")
	if err == nil {
		t.Fatalf("Create error = nil, want non-nil")
	}
	if link != nil {
		t.Fatalf("Create link = %+v, want nil", link)
	}
	if !errors.Is(err, cairn.ErrDestinationRejected) {
		t.Fatalf("error = %v, want errors.Is(_, ErrDestinationRejected)", err)
	}
	if !fired {
		t.Fatalf("OnReject was not fired")
	}
	if rejected.Reason != cairn.ReasonOwnDomain {
		t.Fatalf("RejectEvent.Reason = %q, want %q", rejected.Reason, cairn.ReasonOwnDomain)
	}
	if fs.saveCallCount() != 0 {
		t.Fatalf("Save was called %d times, want 0 (denied before ever reaching the store)", fs.saveCallCount())
	}
}

func TestCreate_InterstitialIsMarkedOnTheRecord(t *testing.T) {
	fs := newFakeStore()
	s, err := cairn.New(fs, cairn.WithPolicy(stubPolicy{decision: cairn.Interstitial, reason: cairn.ReasonNotAllowlisted}))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	link, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v, want nil", err)
	}
	if !link.Interstitial {
		t.Fatalf("Interstitial = false, want true")
	}
}

func TestCreate_FiresOnCreate(t *testing.T) {
	var got cairn.CreateEvent
	fired := false
	fs := newFakeStore()
	s, err := cairn.New(fs, cairn.WithPolicy(allowPolicy{}),
		cairn.WithHooks(cairn.Hooks{OnCreate: func(_ context.Context, ev cairn.CreateEvent) { fired = true; got = ev }}),
	)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	link, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if !fired {
		t.Fatalf("OnCreate was not fired")
	}
	if got.Code != link.Code || got.OwnerID != "user-1" {
		t.Fatalf("CreateEvent = %+v, want Code=%q OwnerID=user-1", got, link.Code)
	}
}

// --- Dedup ---

// Off by default: SR-17 exists because dedup is an existence oracle, and
// that only holds if a caller must opt in explicitly.
//
// Negative control: this test was run against a build of Create with the
// s.dedup gate on the lookup step forced to true. It failed -- a nil-pointer
// panic on s.destIndex, since New only sets destIndex when dedup is enabled;
// the point holds regardless of failure shape: unconditional dedup breaks
// exactly the callers this test represents, who never opted in. Restored
// immediately after.
func TestCreate_DedupIsOffByDefault(t *testing.T) {
	fs := newFakeStore()
	di := fakeDestIndex{fs}
	s, err := cairn.New(di, cairn.WithPolicy(allowPolicy{})) // no WithDeduplication

	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	first, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("first Create error = %v", err)
	}
	second, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("second Create error = %v", err)
	}
	if second.Code == first.Code {
		t.Fatalf("second.Code = %q, same as first %q, want two distinct codes (dedup is off by default)", second.Code, first.Code)
	}
	if fs.saveCallCount() != 2 {
		t.Fatalf("Save called %d times, want 2 (no dedup lookup should have short-circuited the second)", fs.saveCallCount())
	}
}

// A dedup hit must not extend the existing link's life: silently applying a
// later create call's TTL to someone else's earlier link is an ownership
// violation (ADR-0013).
func TestCreate_DedupHitDoesNotModifyTheExistingLinksTTL(t *testing.T) {
	fs := newFakeStore()
	di := fakeDestIndex{fs}
	s, err := cairn.New(di, cairn.WithPolicy(allowPolicy{}), cairn.WithDeduplication(true))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	first, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("first Create error = %v", err)
	}
	if !first.ExpiresAt.IsZero() {
		t.Fatalf("first.ExpiresAt = %v, want zero (no TTL requested)", first.ExpiresAt)
	}

	second, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"), cairn.WithTTL(time.Hour))
	if err != nil {
		t.Fatalf("second Create error = %v", err)
	}
	if second.Code != first.Code {
		t.Fatalf("second.Code = %q, want %q (dedup hit)", second.Code, first.Code)
	}
	if !second.ExpiresAt.IsZero() {
		t.Fatalf("second.ExpiresAt = %v, want zero: the requested TTL must not modify the existing link", second.ExpiresAt)
	}
}

func TestCreate_DedupReturnsExistingLinkOnHit(t *testing.T) {
	fs := newFakeStore()
	di := fakeDestIndex{fs}
	s, err := cairn.New(di, cairn.WithPolicy(allowPolicy{}), cairn.WithDeduplication(true))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	first, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("first Create error = %v", err)
	}
	second, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-1"))
	if err != nil {
		t.Fatalf("second Create error = %v", err)
	}
	if second.Code != first.Code {
		t.Fatalf("second.Code = %q, want %q (dedup hit)", second.Code, first.Code)
	}
	if fs.saveCallCount() != 1 {
		t.Fatalf("Save called %d times, want 1 (second call should hit dedup, not save again)", fs.saveCallCount())
	}
}

func TestCreate_DedupIsScopedPerOwner(t *testing.T) {
	fs := newFakeStore()
	di := fakeDestIndex{fs}
	s, err := cairn.New(di, cairn.WithPolicy(allowPolicy{}), cairn.WithDeduplication(true))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	a, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-a"))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	b, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-b"))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if a.Code == b.Code {
		t.Fatalf("different owners got the same code %q, want dedup scoped per owner", a.Code)
	}
}

// This is one of the two load-bearing orderings issue #28 names explicitly:
// Save must happen before dedup indexing. A store whose Save always fails
// must never see an IndexDest call.
func TestCreate_SavesBeforeIndexingForDedup(t *testing.T) {
	fs := newFakeStore()
	fs.saveErr = errors.New("store down")
	di := &countingDestIndex{fakeDestIndex: fakeDestIndex{fs}}
	s, err := cairn.New(di, cairn.WithPolicy(allowPolicy{}), cairn.WithDeduplication(true))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	_, err = s.Create(context.Background(), "https://example.com/")
	if err == nil {
		t.Fatalf("Create error = nil, want non-nil (store is down)")
	}
	if di.indexCalls != 0 {
		t.Fatalf("IndexDest was called %d times, want 0 (Save failed, so indexing must not happen)", di.indexCalls)
	}
}

type countingDestIndex struct {
	fakeDestIndex
	indexCalls int
}

func (d *countingDestIndex) IndexDest(ctx context.Context, ownerID string, dest cairn.Destination, c cairn.Code) error {
	d.indexCalls++
	return d.fakeDestIndex.IndexDest(ctx, ownerID, dest, c)
}

// --- Vanity codes ---

func TestCreate_VanityCode_UsesTheRequestedCode(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(4, 8, nil))
	link, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("launch")))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if link.Code != "launch" {
		t.Fatalf("Code = %q, want %q", link.Code, "launch")
	}
	if !link.Vanity {
		t.Fatalf("Vanity = false, want true")
	}
}

func TestCreate_VanityCode_RejectsLengthEqualToGeneratedLength(t *testing.T) {
	fs := newFakeStore()
	// The vanity range [4, 8] does not overlap the generated length 10 --
	// New requires that (SR-04) -- but a caller can still submit a
	// vanity code whose length happens to equal 10, and that must be
	// rejected regardless of the configured range.
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(4, 8, nil))
	_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("abcdefghij"))) // len 10
	if !errors.Is(err, cairn.ErrVanityLength) {
		t.Fatalf("error = %v, want errors.Is(_, ErrVanityLength)", err)
	}
}

// Zero store round trips: a generated-length candidate is rejected on length
// alone, before any lookup, so submitting one returns nothing about
// occupancy of the generated space (SR-04, ADR-0011).
func TestCreate_VanityCode_GeneratedLengthRejectedWithZeroStoreRoundTrips(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(4, 8, nil))

	_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("abcdefghij"))) // len 10
	if !errors.Is(err, cairn.ErrVanityLength) {
		t.Fatalf("error = %v, want errors.Is(_, ErrVanityLength)", err)
	}
	if fs.saveCallCount() != 0 {
		t.Fatalf("Save called %d times, want 0 (rejected on length, before any store round trip)", fs.saveCallCount())
	}
}

// A default reserved-word list is always in effect, extendable but never
// something a caller has to remember to ask for (ADR-0011): a vanity code
// shadowing the host's own routes is a routing bug that presents as a
// security incident.
func TestCreate_VanityCode_DefaultReservedWordsAreAlwaysRejected(t *testing.T) {
	for _, word := range cairn.DefaultVanityReserved {
		t.Run(word, func(t *testing.T) {
			fs := newFakeStore()
			s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(3, 8, nil)) // no caller-supplied reserved list; range excludes 10
			_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code(word)))
			// Three of the default entries (robots.txt, favicon.ico,
			// .well-known) contain '.', which AlphabetBase62 does not
			// permit, so they are rejected as invalid codes before the
			// reserved-word check ever runs. That is still "never became a
			// live link" -- the property this test is actually about -- so
			// both outcomes count as a pass, distinguished for precision.
			if err == nil {
				t.Fatalf("Create(vanity=%q) error = nil, want a rejection", word)
			}
			if !errors.Is(err, cairn.ErrVanityReserved) && !errors.Is(err, cairn.ErrInvalidCode) {
				t.Fatalf("Create(vanity=%q) error = %v, want ErrVanityReserved or ErrInvalidCode", word, err)
			}
		})
	}
}

// A caller-supplied reserved list is additive, not a replacement for the
// default.
func TestCreate_VanityCode_CallerReservedListIsAdditiveToTheDefault(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(3, 8, []string{"launch"}))

	_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("admin"))) // default-list word
	if !errors.Is(err, cairn.ErrVanityReserved) {
		t.Fatalf("error = %v, want errors.Is(_, ErrVanityReserved) for a default-reserved word", err)
	}

	_, err = s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("launch"))) // caller-supplied word
	if !errors.Is(err, cairn.ErrVanityReserved) {
		t.Fatalf("error = %v, want errors.Is(_, ErrVanityReserved) for the caller-supplied word", err)
	}
}

func TestCreate_VanityCode_RejectsReservedWord(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(3, 8, []string{"admin"}))
	_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("admin")))
	if !errors.Is(err, cairn.ErrVanityReserved) {
		t.Fatalf("error = %v, want errors.Is(_, ErrVanityReserved)", err)
	}
}

// This is the second load-bearing ordering from issue #28: retrying a vanity
// code would silently issue a different code than the caller asked for, so a
// vanity collision must return ErrCodeExists directly, with exactly one Save
// attempt.
func TestCreate_VanityCode_CollisionIsNotRetried(t *testing.T) {
	fs := newFakeStore()
	fs.saveErr = cairn.ErrCodeExists
	s := newShortener(t, fs, cairn.WithCodeLength(10), cairn.WithVanity(4, 8, nil), cairn.WithMaxSaveAttempts(5))

	_, err := s.Create(context.Background(), "https://example.com/", cairn.WithVanityCode(cairn.Code("launch")))
	if !errors.Is(err, cairn.ErrCodeExists) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeExists)", err)
	}
	if fs.saveCallCount() != 1 {
		t.Fatalf("Save called %d times, want exactly 1 (no retry for a vanity code)", fs.saveCallCount())
	}
}

// --- Generated-code retry (SR-18, SR-19) ---

func TestCreate_RetriesOnCollisionUpToMaxSaveAttempts(t *testing.T) {
	fs := newFakeStore()
	fs.saveErr = cairn.ErrCodeExists
	var retries []cairn.RetryEvent
	s := newShortener(t, fs, cairn.WithMaxSaveAttempts(3),
		cairn.WithHooks(cairn.Hooks{OnRetry: func(_ context.Context, ev cairn.RetryEvent) { retries = append(retries, ev) }}),
	)

	_, err := s.Create(context.Background(), "https://example.com/")
	if !errors.Is(err, cairn.ErrCodeSpaceExhausted) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeSpaceExhausted)", err)
	}
	if fs.saveCallCount() != 3 {
		t.Fatalf("Save called %d times, want exactly 3 (bounded by WithMaxSaveAttempts)", fs.saveCallCount())
	}
	if len(retries) != 3 {
		t.Fatalf("OnRetry fired %d times, want 3 (once per attempt)", len(retries))
	}
	for i, ev := range retries {
		if ev.Attempt != i+1 {
			t.Fatalf("retries[%d].Attempt = %d, want %d", i, ev.Attempt, i+1)
		}
	}
}

func TestCreate_SucceedsAfterACollisionOnAnEarlierAttempt(t *testing.T) {
	fs := newFakeStore()
	attempts := 0
	s := newShortener(t, &firstAttemptFailsStore{fakeStore: fs, failFirstNAttempts: 1, attempts: &attempts})

	link, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v, want nil", err)
	}
	if link == nil {
		t.Fatalf("Create returned a nil link with a nil error")
	}
	if attempts != 2 {
		t.Fatalf("Save called %d times, want 2 (one collision, then success)", attempts)
	}
}

type firstAttemptFailsStore struct {
	*fakeStore
	failFirstNAttempts int
	attempts           *int
}

func (s *firstAttemptFailsStore) Save(ctx context.Context, l *cairn.Link) error {
	*s.attempts++
	if *s.attempts <= s.failFirstNAttempts {
		return cairn.ErrCodeExists
	}
	return s.fakeStore.Save(ctx, l)
}

// Negative control for the retry bound: with createGenerated's loop
// temporarily changed from `attempt <= s.maxSaveAttempts` to an unconditional
// `true`, this exact test failed -- Create did not return within the 2s
// watchdog below, confirming the loop is genuinely unbounded without the
// check. The loop condition was restored immediately after. This test
// asserts the bounded (correct) behavior terminates promptly.
func TestCreate_RetryTerminatesPromptlyRatherThanLooping(t *testing.T) {
	fs := newFakeStore()
	fs.saveErr = cairn.ErrCodeExists
	s := newShortener(t, fs, cairn.WithMaxSaveAttempts(5))

	done := make(chan error, 1)
	go func() {
		_, err := s.Create(context.Background(), "https://example.com/")
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, cairn.ErrCodeSpaceExhausted) {
			t.Fatalf("error = %v, want errors.Is(_, ErrCodeSpaceExhausted)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Create did not return within 2s; retry loop appears unbounded")
	}
}

// SR-20: a non-collision store error must yield no link and must not fire
// OnCreate -- a hook claiming success for a create that failed would be a
// lie the host has no way to detect.
//
// Negative control: this test was run against a build of createGenerated
// with the `!errors.Is(err, ErrCodeExists)` early return removed, so any
// Save error retried like a collision. It failed -- the error surfaced as
// ErrCodeSpaceExhausted after exhausting retries, not ErrStoreUnavailable.
// Restored immediately after.
func TestCreate_StoreErrorYieldsNoLinkAndNoSuccessHook(t *testing.T) {
	fs := newFakeStore()
	fs.saveErr = cairn.ErrStoreUnavailable
	onCreateFired := false
	s := newShortener(t, fs, cairn.WithHooks(cairn.Hooks{OnCreate: func(context.Context, cairn.CreateEvent) { onCreateFired = true }}))

	link, err := s.Create(context.Background(), "https://example.com/")
	if !errors.Is(err, cairn.ErrStoreUnavailable) {
		t.Fatalf("error = %v, want errors.Is(_, ErrStoreUnavailable)", err)
	}
	if link != nil {
		t.Fatalf("link = %+v, want nil", link)
	}
	if onCreateFired {
		t.Fatalf("OnCreate fired for a failed create")
	}
	if fs.saveCallCount() != 1 {
		t.Fatalf("Save called %d times, want exactly 1 (a non-collision error must not retry)", fs.saveCallCount())
	}
}

// --- Normalization and TTL ---

func TestCreate_NormalizesTheDestination(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)
	link, err := s.Create(context.Background(), "HTTP://Example.com:80/path")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if got, want := link.Dest.Raw(), "http://example.com/path"; got != want {
		t.Fatalf("Dest.Raw() = %q, want %q", got, want)
	}
}

func TestCreate_AppliesDefaultTTL(t *testing.T) {
	fixedNow := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithDefaultTTL(time.Hour), cairn.WithClock(func() time.Time { return fixedNow }))
	link, err := s.Create(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if !link.ExpiresAt.Equal(fixedNow.Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %v, want %v", link.ExpiresAt, fixedNow.Add(time.Hour))
	}
}

func TestCreate_WithExpiresAtOverridesDefaultTTL(t *testing.T) {
	explicit := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	fs := newFakeStore()
	s := newShortener(t, fs, cairn.WithDefaultTTL(time.Hour))
	link, err := s.Create(context.Background(), "https://example.com/", cairn.WithExpiresAt(explicit))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if !link.ExpiresAt.Equal(explicit) {
		t.Fatalf("ExpiresAt = %v, want %v", link.ExpiresAt, explicit)
	}
}
