package redisstore

import (
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func TestLinkKey_IsNamespacedAndVersioned(t *testing.T) {
	got := linkKey("abc1234567")
	want := "cairn:v1:link:abc1234567"
	if got != want {
		t.Fatalf("linkKey = %q, want %q", got, want)
	}
}

func TestOwnerKey_IsNamespacedAndVersioned(t *testing.T) {
	got := ownerKey("user-1")
	want := "cairn:v1:owner:user-1"
	if got != want {
		t.Fatalf("ownerKey = %q, want %q", got, want)
	}
}

func TestHitsKey_IsNamespacedAndVersioned(t *testing.T) {
	got := hitsKey("abc1234567")
	want := "cairn:v1:hits:abc1234567"
	if got != want {
		t.Fatalf("hitsKey = %q, want %q", got, want)
	}
}

// The dedup index keys on a hash, never on the destination text itself
// (SR-15): a key name shows up in SCAN output, MONITOR and slow-query logs,
// none of which are covered by Destination's redaction.
func TestDestKey_NeverContainsTheDestinationText(t *testing.T) {
	dest, err := cairn.ParseDestination("https://example.com/reset?token=s3cr3t")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	got := destKey("user-1", dest)
	if !strings.HasPrefix(got, "cairn:v1:dest:") {
		t.Fatalf("destKey = %q, want prefix %q", got, "cairn:v1:dest:")
	}
	if strings.Contains(got, "s3cr3t") || strings.Contains(got, "example.com") {
		t.Fatalf("destKey = %q, leaks the destination", got)
	}
}

func TestDestKey_IsScopedPerOwner(t *testing.T) {
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	a := destKey("user-a", dest)
	b := destKey("user-b", dest)
	if a == b {
		t.Fatalf("destKey is the same for two owners: %q", a)
	}
}

func TestDestKey_IsDeterministic(t *testing.T) {
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	first := destKey("user-1", dest)
	second := destKey("user-1", dest)
	if first != second {
		t.Fatalf("destKey is not deterministic: %q != %q", first, second)
	}
}

// No key is built from an unvalidated code -- asserted, not assumed: a code
// containing characters that could look like a namespace delimiter must
// still produce a distinct, correctly round-trippable key rather than
// colliding with another namespace or another code.
func TestLinkKey_DoesNotCollideAcrossUnusualCodes(t *testing.T) {
	a := linkKey(cairn.Code("ab:cd"))
	b := linkKey(cairn.Code("ab"))
	if a == b {
		t.Fatalf("linkKey(%q) and linkKey(%q) collided: %q", "ab:cd", "ab", a)
	}
}
