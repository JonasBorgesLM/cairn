package cairn_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func destForHooks(t *testing.T) cairn.Destination {
	t.Helper()
	d, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	return d
}

// A nil field on Hooks is a no-op: calling the corresponding hook must not
// panic when the field was never set.
func TestHooks_NilFieldsAreNoOps(t *testing.T) {
	var h cairn.Hooks
	ctx := context.Background()

	if h.OnCreate != nil {
		h.OnCreate(ctx, cairn.CreateEvent{})
	}
	if h.OnResolve != nil {
		h.OnResolve(ctx, cairn.ResolveEvent{})
	}
	if h.OnReject != nil {
		h.OnReject(ctx, cairn.RejectEvent{})
	}
	if h.OnRetry != nil {
		h.OnRetry(ctx, cairn.RetryEvent{})
	}
	// Reaching this line without a panic is the assertion.
}

func TestHooks_OnCreate_IsCalledWithTheEvent(t *testing.T) {
	dest := destForHooks(t)
	var got cairn.CreateEvent
	called := false

	h := cairn.Hooks{
		OnCreate: func(_ context.Context, ev cairn.CreateEvent) {
			called = true
			got = ev
		},
	}

	want := cairn.CreateEvent{Code: cairn.Code("abc123"), Dest: dest, OwnerID: "user-1"}
	h.OnCreate(context.Background(), want)

	if !called {
		t.Fatalf("OnCreate was not invoked")
	}
	if got.Code != want.Code || !got.Dest.Equal(want.Dest) || got.OwnerID != want.OwnerID {
		t.Fatalf("OnCreate received %+v, want %+v", got, want)
	}
}

func TestHooks_OnReject_CarriesDestinationAndReason(t *testing.T) {
	dest := destForHooks(t)
	var got cairn.RejectEvent

	h := cairn.Hooks{
		OnReject: func(_ context.Context, ev cairn.RejectEvent) {
			got = ev
		},
	}

	h.OnReject(context.Background(), cairn.RejectEvent{Dest: dest, Reason: cairn.ReasonScheme})

	if !got.Dest.Equal(dest) {
		t.Fatalf("RejectEvent.Dest mismatch")
	}
	if got.Reason != cairn.ReasonScheme {
		t.Fatalf("RejectEvent.Reason = %q, want %q", got.Reason, cairn.ReasonScheme)
	}
}

func TestHooks_OnResolve_CarriesCodeAndDestination(t *testing.T) {
	dest := destForHooks(t)
	var got cairn.ResolveEvent

	h := cairn.Hooks{
		OnResolve: func(_ context.Context, ev cairn.ResolveEvent) {
			got = ev
		},
	}

	h.OnResolve(context.Background(), cairn.ResolveEvent{Code: cairn.Code("abc123"), Dest: dest})

	if got.Code != "abc123" {
		t.Fatalf("ResolveEvent.Code = %q, want %q", got.Code, "abc123")
	}
	if !got.Dest.Equal(dest) {
		t.Fatalf("ResolveEvent.Dest mismatch")
	}
}

func TestHooks_OnRetry_CarriesCodeAndAttempt(t *testing.T) {
	var got cairn.RetryEvent

	h := cairn.Hooks{
		OnRetry: func(_ context.Context, ev cairn.RetryEvent) {
			got = ev
		},
	}

	h.OnRetry(context.Background(), cairn.RetryEvent{Code: cairn.Code("abc123"), Attempt: 2})

	if got.Code != "abc123" || got.Attempt != 2 {
		t.Fatalf("RetryEvent = %+v, want {Code: abc123, Attempt: 2}", got)
	}
}

// No event struct may carry a raw URL string: SR-15's guarantee only holds if
// every field that can name a destination is itself a Destination, which
// redacts, and never a string, which does not.
func TestEventStructs_CarryNoRawStringDestinationField(t *testing.T) {
	events := []any{
		cairn.CreateEvent{},
		cairn.ResolveEvent{},
		cairn.RejectEvent{},
		cairn.RetryEvent{},
	}

	for _, ev := range events {
		typ := reflect.TypeOf(ev)
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Type.Kind() == reflect.String && looksLikeADestinationField(f.Name) {
					t.Fatalf("field %s.%s is a string that looks like a destination field", typ.Name(), f.Name)
				}
			}
		})
	}
}

func looksLikeADestinationField(name string) bool {
	switch name {
	case "Dest", "Destination", "URL", "RawURL", "Raw":
		return true
	default:
		return false
	}
}
