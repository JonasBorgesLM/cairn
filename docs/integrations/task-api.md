# task-api: cairn as embedded Service-layer domain logic

**IR-04.** `task-api` is the hypothetical first real consumer
[`docs/INTEGRATION.md`](../INTEGRATION.md) §4 already names. This document is
the deeper dive on the one integration decision that matters more than the
rest combined: the ownership check. Read `docs/INTEGRATION.md` §4 first for
the layering and the error envelope; this document does not repeat those.

This is a design, not shipped code — `task-api` is not a dependency here, and
never will be one, the same way `crier`'s own `docs/integrations/task-api.md`
is prose rather than a vendored consumer. Unlike `crier`'s version, though,
the pattern below is backed by a real, running test in this repository —
[`example_ownership_test.go`](../../example_ownership_test.go) — because the
bug this document exists to prevent is exactly the kind that looks fine until
someone writes the test that checks for it.

## The one integration bug that matters

**`Shortener.Revoke` performs no ownership check, by design (ADR-0010).**
`Revoke` takes a code and revokes it. It does not take, does not ask for, and
does not verify a caller identity, because ownership is a fact about the
*host's* users, and cairn has no model of a user at all — baking a check in
would mean guessing at an authorization model that fits some hosts and is
wrong for others.

The consequence is that the check does not happen by accident. A `task-api`
handler that calls `shortener.Revoke(ctx, code)` directly, without first
confirming the authenticated principal owns that code, lets any authenticated
user revoke any other user's link. It compiles. It passes a test that only
checks "revoking a link I own works." It is the single most likely
integration bug in a project built on cairn, and it is the kind that survives
code review, because the code that is missing is the code nobody wrote.

## The pattern

The check lives in the Service layer, between the HTTP handler and cairn:

```go
// service wraps *cairn.Shortener with the ownership check ADR-0010 says
// cairn itself must not perform.
type service struct {
    shortener *cairn.Shortener
}

var errNotOwner = errors.New("task-api: principal does not own this link")

// revokeAsOwner loads the link through Resolve -- which already fails
// exactly as SR-03 requires for a code that is invalid, unknown, expired or
// already revoked -- compares OwnerID against the authenticated principal,
// and only then revokes.
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
```

Using `Resolve` rather than a separate lookup is deliberate, not incidental:
it means an already-revoked, expired, or nonexistent code is rejected with the
same uniform shape SR-03 already requires for the read path, before ownership
is even checked — a caller probing for codes by trying to revoke them learns
nothing more from this endpoint than they would from `GET /{code}`.

`task-api`'s actual HTTP handler maps `errNotOwner` to whatever shape a
caller who does not own a resource should see — most naturally the same `404`
the anonymous path already uses (SR-16's own reasoning: telling an
unauthorized caller "this belongs to someone else" is itself a disclosure a
scanner can use, so `task-api`'s handler should not distinguish
"not yours" from "not found" any more than cairn's own default distinguishes
"not found" from "revoked").

## The test

[`TestServiceLayer_RevokeAsOwner_RejectsWrongOwner`](../../example_ownership_test.go)
is the dedicated negative test both this document and `docs/INTEGRATION.md`
§4.1 call for: it creates a link owned by `user-a`, confirms `user-b` cannot
revoke it (and that the rejected attempt left the link untouched), then
confirms `user-a` can. Its own negative control — run once, against a build
of `revokeAsOwner` with the `OwnerID` comparison removed, restored
immediately after — is recorded in the comment above the test: the attempt by
`user-b` succeeded, which is exactly the vulnerability this pattern exists to
close.

A real `task-api` deployment adapts this at the boundary that knows about
authentication (a bearer token, a session), not inside cairn, and not inside
this test — the test's job is to prove the *shape* of the check is correct,
which is the part a host can get wrong regardless of how it authenticates.
