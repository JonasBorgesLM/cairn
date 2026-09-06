package cairn_test

import (
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

func TestLink_IsExpired_ZeroExpiresAtMeansNever(t *testing.T) {
	l := &cairn.Link{}
	if l.IsExpired(time.Now()) {
		t.Fatalf("IsExpired() = true for zero ExpiresAt, want false")
	}
	// Not even a very distant "now" expires it.
	if l.IsExpired(time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("IsExpired(far future) = true for zero ExpiresAt, want false")
	}
}

func TestLink_IsExpired_AtTheExactBoundaryInstant(t *testing.T) {
	boundary := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	l := &cairn.Link{ExpiresAt: boundary}

	if !l.IsExpired(boundary) {
		t.Fatalf("IsExpired(boundary) = false, want true (expiry is inclusive)")
	}
	if l.IsExpired(boundary.Add(-time.Nanosecond)) {
		t.Fatalf("IsExpired(boundary - 1ns) = true, want false")
	}
	if !l.IsExpired(boundary.Add(time.Nanosecond)) {
		t.Fatalf("IsExpired(boundary + 1ns) = false, want true")
	}
}

func TestLink_IsRevoked(t *testing.T) {
	l := &cairn.Link{}
	if l.IsRevoked() {
		t.Fatalf("IsRevoked() = true for zero RevokedAt, want false")
	}

	l.RevokedAt = time.Now()
	if !l.IsRevoked() {
		t.Fatalf("IsRevoked() = false after setting RevokedAt, want true")
	}
}

func TestLink_IsActive(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		link *cairn.Link
		want bool
	}{
		{"neither expired nor revoked", &cairn.Link{}, true},
		{"expired", &cairn.Link{ExpiresAt: now.Add(-time.Hour)}, false},
		{"revoked", &cairn.Link{RevokedAt: now.Add(-time.Hour)}, false},
		{"expired and revoked", &cairn.Link{ExpiresAt: now.Add(-time.Hour), RevokedAt: now.Add(-time.Minute)}, false},
		{"not yet expired", &cairn.Link{ExpiresAt: now.Add(time.Hour)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.link.IsActive(now); got != tt.want {
				t.Fatalf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLink_Fields(t *testing.T) {
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	now := time.Now()

	l := &cairn.Link{
		Code:      cairn.Code("abc123"),
		Dest:      dest,
		OwnerID:   "user-1",
		Vanity:    true,
		CreatedAt: now,
	}

	if l.Code != "abc123" {
		t.Fatalf("Code = %q, want %q", l.Code, "abc123")
	}
	if !l.Dest.Equal(dest) {
		t.Fatalf("Dest mismatch")
	}
	if l.OwnerID != "user-1" {
		t.Fatalf("OwnerID = %q, want %q", l.OwnerID, "user-1")
	}
	if !l.Vanity {
		t.Fatalf("Vanity = false, want true")
	}
	if !l.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", l.CreatedAt, now)
	}
}
