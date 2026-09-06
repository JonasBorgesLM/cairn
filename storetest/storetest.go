// Package storetest is a shared conformance suite for cairn.Store
// implementations. memstore and redisstore both run it, so the one contract
// that must not vary between them — Save's conditional-write behavior
// (SR-18) — is asserted once and enforced identically everywhere, rather than
// reimplemented per store and drifting.
//
// It lives in the core module, not inside memstore, so that redisstore — a
// separate module — can import it without memstore becoming a dependency of
// either.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JonasBorgesLM/cairn"
)

// testCode is the code most subtests save under; a shared constant since
// goconst would otherwise flag the literal repeated across every subtest.
const testCode cairn.Code = "abc1234567"

// Run exercises the Store contract against a fresh store returned by newStore
// for each subtest. newStore must return an empty store every time it is
// called; Run does not assume any relationship between successive calls.
func Run(t *testing.T, newStore func() cairn.Store) {
	t.Helper()

	t.Run("SaveThenLoadRoundTrips", func(t *testing.T) { testSaveThenLoad(t, newStore()) })
	t.Run("SaveIsConditional", func(t *testing.T) { testSaveIsConditional(t, newStore()) })
	t.Run("LoadMissingReturnsErrCodeNotFound", func(t *testing.T) { testLoadMissing(t, newStore()) })
	t.Run("RevokeSetsRevokedAt", func(t *testing.T) { testRevoke(t, newStore()) })
	t.Run("RevokeWithPurgeClearsDestination", func(t *testing.T) { testRevokePurge(t, newStore()) })
	t.Run("RevokeUnknownCodeReturnsErrCodeNotFound", func(t *testing.T) { testRevokeMissing(t, newStore()) })
}

func mustDest(t *testing.T, raw string) cairn.Destination {
	t.Helper()
	d, err := cairn.ParseDestination(raw)
	if err != nil {
		t.Fatalf("ParseDestination(%q) error = %v", raw, err)
	}
	return d
}

func testSaveThenLoad(t *testing.T, store cairn.Store) {
	dest := mustDest(t, "https://example.com/path")
	link := &cairn.Link{Code: testCode, Dest: dest, OwnerID: "user-1", CreatedAt: time.Now()}

	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v, want nil", err)
	}

	got, err := store.Load(context.Background(), testCode)
	if err != nil {
		t.Fatalf("Load error = %v, want nil", err)
	}
	if got.Code != link.Code {
		t.Fatalf("Code = %q, want %q", got.Code, link.Code)
	}
	if !got.Dest.Equal(link.Dest) {
		t.Fatalf("Dest mismatch after round trip")
	}
	if got.OwnerID != link.OwnerID {
		t.Fatalf("OwnerID = %q, want %q", got.OwnerID, link.OwnerID)
	}
}

// testSaveIsConditional is the center of this suite: SR-18 depends on it.
func testSaveIsConditional(t *testing.T, store cairn.Store) {
	dest := mustDest(t, "https://example.com/original")
	original := &cairn.Link{Code: testCode, Dest: dest, CreatedAt: time.Now()}
	if err := store.Save(context.Background(), original); err != nil {
		t.Fatalf("first Save error = %v, want nil", err)
	}

	attacker := &cairn.Link{Code: testCode, Dest: mustDest(t, "https://evil.example/"), CreatedAt: time.Now()}
	err := store.Save(context.Background(), attacker)
	if !errors.Is(err, cairn.ErrCodeExists) {
		t.Fatalf("second Save error = %v, want errors.Is(_, ErrCodeExists)", err)
	}

	got, loadErr := store.Load(context.Background(), testCode)
	if loadErr != nil {
		t.Fatalf("Load error = %v", loadErr)
	}
	if !got.Dest.Equal(original.Dest) {
		t.Fatalf("record was overwritten by the second Save: Save must be conditional (SR-18)")
	}
}

func testLoadMissing(t *testing.T, store cairn.Store) {
	_, err := store.Load(context.Background(), "doesnotexist")
	if !errors.Is(err, cairn.ErrCodeNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeNotFound)", err)
	}
}

func testRevoke(t *testing.T, store cairn.Store) {
	link := &cairn.Link{Code: testCode, Dest: mustDest(t, "https://example.com/"), CreatedAt: time.Now()}
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	at := time.Now()
	if err := store.Revoke(context.Background(), testCode, at, false); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	got, err := store.Load(context.Background(), testCode)
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if !got.IsRevoked() {
		t.Fatalf("IsRevoked() = false after Revoke, want true")
	}
	if got.Dest.IsZero() {
		t.Fatalf("Dest.IsZero() = true after a non-purging Revoke, want the destination kept")
	}
}

func testRevokePurge(t *testing.T, store cairn.Store) {
	link := &cairn.Link{Code: testCode, Dest: mustDest(t, "https://example.com/"), CreatedAt: time.Now()}
	if err := store.Save(context.Background(), link); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	if err := store.Revoke(context.Background(), testCode, time.Now(), true); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}

	got, err := store.Load(context.Background(), testCode)
	if err != nil {
		t.Fatalf("Load error = %v, want nil (the record must still exist)", err)
	}
	if !got.IsRevoked() {
		t.Fatalf("IsRevoked() = false, want true")
	}
	if !got.Dest.IsZero() {
		t.Fatalf("Dest.IsZero() = false after a purging Revoke, want the destination cleared")
	}
}

func testRevokeMissing(t *testing.T, store cairn.Store) {
	err := store.Revoke(context.Background(), "doesnotexist", time.Now(), false)
	if !errors.Is(err, cairn.ErrCodeNotFound) {
		t.Fatalf("error = %v, want errors.Is(_, ErrCodeNotFound)", err)
	}
}
