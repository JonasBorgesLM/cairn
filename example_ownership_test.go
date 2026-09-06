package cairn_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// service is the Service-layer wrapper docs/integrations/task-api.md
// describes: ADR-0010 is explicit that cairn.Revoke performs no ownership
// check by design, so a host exposing revocation to end users must add one
// itself, before calling it -- and this is the pattern that does that.
type service struct {
	shortener *cairn.Shortener
}

var errNotOwner = errors.New("task-api: principal does not own this link")

// revokeAsOwner loads the link through Resolve (which cairn already
// validates and which fails exactly as SR-03 requires for a code that is
// invalid, unknown, expired or already revoked), compares OwnerID against
// the authenticated principal, and only then revokes.
func (s *service) revokeAsOwner(ctx context.Context, code cairn.Code, principalID string) error {
	link, err := s.shortener.Resolve(ctx, code)
	if err != nil {
		return err
	}
	if link.OwnerID != principalID {
		return errNotOwner
	}
	return s.shortener.Revoke(ctx, code)
}

// TestServiceLayer_RevokeAsOwner_RejectsWrongOwner is the dedicated negative
// test docs/INTEGRATION.md §4.1 and docs/integrations/task-api.md both call
// for: user A cannot revoke user B's link. cairn itself performs no such
// check (ADR-0010) -- this proves the Service-layer wrapper above does.
//
// Negative control: this test was run against a build of revokeAsOwner with
// the `if link.OwnerID != principalID` check removed. It failed -- user B's
// revocation attempt succeeded, and the link came back ErrLinkRevoked to
// user A's own subsequent Resolve. Restored immediately after.
func TestServiceLayer_RevokeAsOwner_RejectsWrongOwner(t *testing.T) {
	s := newShortener(t, newFakeStore())
	svc := &service{shortener: s}

	link, err := s.Create(context.Background(), "https://example.com/", cairn.WithOwner("user-a"))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	if err := svc.revokeAsOwner(context.Background(), link.Code, "user-b"); !errors.Is(err, errNotOwner) {
		t.Fatalf("revokeAsOwner(user-b) error = %v, want errNotOwner", err)
	}

	// The rejected attempt must not have revoked it regardless.
	if _, err := s.Resolve(context.Background(), link.Code); err != nil {
		t.Fatalf("Resolve after rejected revoke attempt error = %v, want nil (still active)", err)
	}

	if err := svc.revokeAsOwner(context.Background(), link.Code, "user-a"); err != nil {
		t.Fatalf("revokeAsOwner(user-a) error = %v, want nil", err)
	}
	if _, err := s.Resolve(context.Background(), link.Code); !errors.Is(err, cairn.ErrLinkRevoked) {
		t.Fatalf("Resolve after correct-owner revoke error = %v, want errors.Is(_, ErrLinkRevoked)", err)
	}
}
