package cairn_test

import (
	"context"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// These types exist purely so the compiler pins the interfaces' shapes:
// a change to any method signature here breaks the build, which is the
// point.

type storeStub struct{}

func (storeStub) Save(context.Context, *cairn.Link) error                   { return nil }
func (storeStub) Load(context.Context, cairn.Code) (*cairn.Link, error)     { return nil, nil }
func (storeStub) Revoke(context.Context, cairn.Code, time.Time, bool) error { return nil }

var _ cairn.Store = storeStub{}

type ownerListerStub struct{ storeStub }

func (ownerListerStub) ListByOwner(context.Context, string, cairn.Cursor, int) ([]*cairn.Link, cairn.Cursor, error) {
	return nil, "", nil
}

var _ cairn.OwnerLister = ownerListerStub{}

type destIndexStub struct{ storeStub }

func (destIndexStub) LookupByDest(context.Context, string, cairn.Destination) (cairn.Code, error) {
	return "", nil
}
func (destIndexStub) IndexDest(context.Context, string, cairn.Destination, cairn.Code) error {
	return nil
}

var _ cairn.DestIndex = destIndexStub{}

type evictionCheckerStub struct{ storeStub }

func (evictionCheckerStub) EvictionCheck(context.Context) error { return nil }

var _ cairn.EvictionChecker = evictionCheckerStub{}

type counterStub struct{}

func (counterStub) Count(context.Context, cairn.Code) error { return nil }

var _ cairn.Counter = counterStub{}
