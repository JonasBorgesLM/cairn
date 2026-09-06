package storetest_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// trivialStore is the minimum correct cairn.Store: a mutex-guarded map with a
// conditional Save. Real implementations (memstore, redisstore) run
// storetest.Run against themselves the same way a store's own test would:
//
//	func TestConformance(t *testing.T) {
//		storetest.Run(t, func() cairn.Store { return New() })
//	}
//
// storetest.Run itself takes a *testing.T, so this example -- which has none
// to give it -- instead exercises the same Store contract it checks, to show
// the shape any implementation must satisfy.
type trivialStore struct {
	mu      sync.Mutex
	records map[cairn.Code]*cairn.Link
}

func (s *trivialStore) Save(_ context.Context, l *cairn.Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = map[cairn.Code]*cairn.Link{}
	}
	if _, exists := s.records[l.Code]; exists {
		return cairn.ErrCodeExists
	}
	cp := *l
	s.records[l.Code] = &cp
	return nil
}

func (s *trivialStore) Load(_ context.Context, c cairn.Code) (*cairn.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.records[c]
	if !ok {
		return nil, cairn.ErrCodeNotFound
	}
	cp := *l
	return &cp, nil
}

func (s *trivialStore) Revoke(_ context.Context, c cairn.Code, at time.Time, purge bool) error {
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

func Example() {
	var store cairn.Store = &trivialStore{}
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		panic(err)
	}

	err = store.Save(context.Background(), &cairn.Link{Code: "abc1234567", Dest: dest})
	fmt.Println("first Save:", err)

	err = store.Save(context.Background(), &cairn.Link{Code: "abc1234567", Dest: dest})
	fmt.Println("second Save:", err)

	// Output:
	// first Save: <nil>
	// second Save: cairn: code already exists
}
