package cairn_test

import (
	"context"
	"sync"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// fakeStore is a minimal, configurable cairn.Store for unit-testing
// Shortener's flows in isolation. The real conformance suite lives with
// memstore; this fake exists to make specific behaviors -- a forced
// collision, a counted call, an injected error -- cheap to set up.
type fakeStore struct {
	mu      sync.Mutex
	records map[cairn.Code]*cairn.Link

	saveErr   error // if set, every Save returns this error
	saveCalls int
	loadCalls int

	// dest indexes a code by (ownerID, raw destination string), for
	// fakeDestIndex below.
	dest map[string]cairn.Code
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: map[cairn.Code]*cairn.Link{}}
}

func (s *fakeStore) Save(_ context.Context, l *cairn.Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	if _, exists := s.records[l.Code]; exists {
		return cairn.ErrCodeExists
	}
	cp := *l
	s.records[l.Code] = &cp
	return nil
}

func (s *fakeStore) Load(_ context.Context, c cairn.Code) (*cairn.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalls++
	l, ok := s.records[c]
	if !ok {
		return nil, cairn.ErrCodeNotFound
	}
	cp := *l
	return &cp, nil
}

func (s *fakeStore) Revoke(_ context.Context, c cairn.Code, at time.Time, purge bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.records[c]
	if !ok {
		return cairn.ErrCodeNotFound
	}
	l.RevokedAt = at
	if purge {
		l.Dest = cairn.Destination{}
	}
	return nil
}

func (s *fakeStore) saveCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCalls
}

func (s *fakeStore) loadCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCalls
}

// fakeDestIndex adds cairn.DestIndex to a fakeStore.
type fakeDestIndex struct{ *fakeStore }

func (s fakeDestIndex) key(ownerID string, d cairn.Destination) string {
	return ownerID + "\x00" + d.Raw()
}

func (s fakeDestIndex) LookupByDest(_ context.Context, ownerID string, d cairn.Destination) (cairn.Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dest == nil {
		return "", cairn.ErrCodeNotFound
	}
	c, ok := s.dest[s.key(ownerID, d)]
	if !ok {
		return "", cairn.ErrCodeNotFound
	}
	return c, nil
}

func (s fakeDestIndex) IndexDest(_ context.Context, ownerID string, d cairn.Destination, c cairn.Code) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dest == nil {
		s.dest = map[string]cairn.Code{}
	}
	s.dest[s.key(ownerID, d)] = c
	return nil
}

// fakeEvictionChecker adds cairn.EvictionChecker to a fakeStore.
type fakeEvictionChecker struct {
	*fakeStore
	err error
}

func (s fakeEvictionChecker) EvictionCheck(context.Context) error { return s.err }

// allowPolicy is a cairn.Policy that always Allows.
type allowPolicy struct{}

func (allowPolicy) Evaluate(context.Context, cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	return cairn.Allow, "", nil
}
