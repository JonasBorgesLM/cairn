package memstore

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// Store is an in-memory cairn.Store, safe for concurrent use. It implements
// cairn.OwnerLister, cairn.DestIndex and cairn.Counter as well as the
// required cairn.Store methods, so a consumer's own tests can exercise every
// optional path without a container.
//
// Nothing here is durable: a process restart loses every record. That trade
// is the whole point — this is a test double, not a deployment target.
type Store struct {
	mu       sync.Mutex
	records  map[cairn.Code]*cairn.Link
	byOwner  map[string][]cairn.Code // creation order, per owner
	byDest   map[string]cairn.Code   // ownerID + "\x00" + raw destination -> code
	counters map[cairn.Code]int
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		records:  make(map[cairn.Code]*cairn.Link),
		byOwner:  make(map[string][]cairn.Code),
		byDest:   make(map[string]cairn.Code),
		counters: make(map[cairn.Code]int),
	}
}

// Save implements cairn.Store. It is conditional: an existing code returns
// cairn.ErrCodeExists and the record is never overwritten (SR-18).
func (s *Store) Save(_ context.Context, l *cairn.Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.records[l.Code]; exists {
		return cairn.ErrCodeExists
	}

	cp := *l
	s.records[l.Code] = &cp
	s.byOwner[l.OwnerID] = append(s.byOwner[l.OwnerID], l.Code)
	return nil
}

// Load implements cairn.Store.
func (s *Store) Load(_ context.Context, c cairn.Code) (*cairn.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.records[c]
	if !ok {
		return nil, cairn.ErrCodeNotFound
	}
	cp := *l
	return &cp, nil
}

// Revoke implements cairn.Store: it sets RevokedAt and, when purgeDestination
// is true, clears Dest in the same update (ADR-0009).
func (s *Store) Revoke(_ context.Context, c cairn.Code, at time.Time, purgeDestination bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.records[c]
	if !ok {
		return cairn.ErrCodeNotFound
	}
	l.RevokedAt = at
	if purgeDestination {
		l.Dest = cairn.Destination{}
	}
	return nil
}

// ListByOwner implements cairn.OwnerLister, paginating in creation order.
func (s *Store) ListByOwner(_ context.Context, ownerID string, after cairn.Cursor, limit int) ([]*cairn.Link, cairn.Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	codes := s.byOwner[ownerID]

	start := 0
	if after != "" {
		n, err := strconv.Atoi(string(after))
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("memstore: invalid cursor %q", after)
		}
		start = n
	}
	start = min(start, len(codes))
	end := min(start+limit, len(codes))

	links := make([]*cairn.Link, 0, end-start)
	for _, c := range codes[start:end] {
		cp := *s.records[c]
		links = append(links, &cp)
	}

	var next cairn.Cursor
	if end < len(codes) {
		next = cairn.Cursor(strconv.Itoa(end))
	}
	return links, next, nil
}

func destKey(ownerID string, d cairn.Destination) string {
	return ownerID + "\x00" + d.Raw()
}

// LookupByDest implements cairn.DestIndex, scoped per ownerID (ADR-0013).
func (s *Store) LookupByDest(_ context.Context, ownerID string, d cairn.Destination) (cairn.Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.byDest[destKey(ownerID, d)]
	if !ok {
		return "", cairn.ErrCodeNotFound
	}
	return c, nil
}

// IndexDest implements cairn.DestIndex, scoped per ownerID (ADR-0013).
func (s *Store) IndexDest(_ context.Context, ownerID string, d cairn.Destination, c cairn.Code) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byDest[destKey(ownerID, d)] = c
	return nil
}

// Count implements cairn.Counter.
func (s *Store) Count(_ context.Context, c cairn.Code) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counters[c]++
	return nil
}

// CountOf returns the number of times Count has been called for c. It exists
// for a consumer's own tests to assert on — memstore has no other way to
// observe what Count recorded.
func (s *Store) CountOf(c cairn.Code) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.counters[c]
}
