package cairn_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func TestRevoke_RejectsInvalidCode(t *testing.T) {
	s := newShortener(t, newFakeStore())
	err := s.Revoke(context.Background(), "has a space")
	if err == nil {
		t.Fatalf("Revoke(invalid code) error = nil, want non-nil")
	}
}

func TestRevoke_KeepsTheDestination(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	link, err := s.Create(context.Background(), "https://example.com/secret?token=abc")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if err = s.Revoke(context.Background(), link.Code); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	stored, err := fs.Load(context.Background(), link.Code)
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if stored.Dest.IsZero() {
		t.Fatalf("Dest.IsZero() = true after plain Revoke, want the destination kept")
	}
	if !stored.IsRevoked() {
		t.Fatalf("IsRevoked() = false, want true")
	}
}

func TestRevokeAndPurge_ClearsTheDestination(t *testing.T) {
	fs := newFakeStore()
	s := newShortener(t, fs)

	link, err := s.Create(context.Background(), "https://example.com/secret?token=abc")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if err = s.RevokeAndPurge(context.Background(), link.Code); err != nil {
		t.Fatalf("RevokeAndPurge error = %v", err)
	}

	stored, err := fs.Load(context.Background(), link.Code)
	if err != nil {
		t.Fatalf("Load error = %v, want nil (the record itself must still exist)", err)
	}
	if !stored.IsRevoked() {
		t.Fatalf("IsRevoked() = false, want true")
	}
	if !stored.Dest.IsZero() {
		t.Fatalf("Dest.IsZero() = false after purge, want the destination cleared")
	}
}

func TestRevoke_NextResolveFailsImmediately(t *testing.T) {
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
