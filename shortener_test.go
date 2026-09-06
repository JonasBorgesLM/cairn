package cairn_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func TestNew_RejectsNilStore(t *testing.T) {
	_, err := cairn.New(nil, cairn.WithPolicy(allowPolicy{}))
	if err == nil {
		t.Fatalf("New(nil store) error = nil, want non-nil")
	}
}

// ADR-0005: a shortener without a destination policy is the thing this
// library exists not to be, so New refuses to construct one.
func TestNew_RejectsNoPolicy(t *testing.T) {
	_, err := cairn.New(newFakeStore())
	if err == nil {
		t.Fatalf("New(no policy) error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "Policy") {
		t.Fatalf("error = %q, want it to name the missing Policy", err.Error())
	}
}

// ADR-0002: WithCodeLength(5) against a million expected links must fail --
// this is the literal scenario from issue #19/#27.
func TestNew_RejectsUnsafeKeyspaceDensity(t *testing.T) {
	_, err := cairn.New(newFakeStore(),
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithCodeLength(5),
		cairn.WithExpectedLinks(1_000_000),
	)
	if err == nil {
		t.Fatalf("New(unsafe density) error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "ADR-0002") {
		t.Fatalf("error = %q, want it to name ADR-0002", err.Error())
	}
}

func TestNew_AllowsASafeKeyspaceDensity(t *testing.T) {
	_, err := cairn.New(newFakeStore(),
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithExpectedLinks(1_000_000), // default length 10 is safe here
	)
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
}

// ADR-0013: dedup requires a Store that implements DestIndex.
func TestNew_RejectsDedupAgainstAStoreWithoutDestIndex(t *testing.T) {
	_, err := cairn.New(newFakeStore(), // fakeStore alone is not a DestIndex
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithDeduplication(true),
	)
	if err == nil {
		t.Fatalf("New(dedup, non-DestIndex store) error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "DestIndex") {
		t.Fatalf("error = %q, want it to name DestIndex", err.Error())
	}
}

func TestNew_AllowsDedupAgainstADestIndexStore(t *testing.T) {
	fs := newFakeStore()
	_, err := cairn.New(fakeDestIndex{fs},
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithDeduplication(true),
	)
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
}

// SR-04: a vanity range that overlaps the generated code length must fail at
// New, not surface as a confusing collision later.
func TestNew_RejectsVanityRangeOverlappingGeneratedLength(t *testing.T) {
	_, err := cairn.New(newFakeStore(),
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithCodeLength(10),
		cairn.WithVanity(5, 12, nil), // 10 is inside [5,12]
	)
	if err == nil {
		t.Fatalf("New(overlapping vanity range) error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "SR-04") {
		t.Fatalf("error = %q, want it to name SR-04", err.Error())
	}
}

func TestNew_AllowsANonOverlappingVanityRange(t *testing.T) {
	_, err := cairn.New(newFakeStore(),
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithCodeLength(10),
		cairn.WithVanity(4, 8, nil),
	)
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
}

// ADR-0008: a store reporting an evicting policy must not be started against.
func TestNew_RejectsAnEvictingStore(t *testing.T) {
	boom := errors.New("maxmemory-policy is allkeys-lru")
	fs := newFakeStore()
	_, err := cairn.New(fakeEvictionChecker{fakeStore: fs, err: boom},
		cairn.WithPolicy(allowPolicy{}),
	)
	if !errors.Is(err, boom) {
		t.Fatalf("New(evicting store) error = %v, want errors.Is(_, boom)", err)
	}
}

func TestNew_AllowsANonEvictingStore(t *testing.T) {
	fs := newFakeStore()
	_, err := cairn.New(fakeEvictionChecker{fakeStore: fs, err: nil},
		cairn.WithPolicy(allowPolicy{}),
	)
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
}

func TestNew_MinimalValidConfiguration(t *testing.T) {
	s, err := cairn.New(newFakeStore(), cairn.WithPolicy(allowPolicy{}))
	if err != nil {
		t.Fatalf("New error = %v, want nil", err)
	}
	if s == nil {
		t.Fatalf("New returned a nil *Shortener with a nil error")
	}
}
