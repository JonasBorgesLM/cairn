package cairn_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

// Every sentinel must stay comparable with errors.Is once wrapped, per NFR-07.
func TestSentinelErrors_ComparableWithErrorsIs(t *testing.T) {
	sentinels := map[string]error{
		"ErrCodeExists":          cairn.ErrCodeExists,
		"ErrCodeNotFound":        cairn.ErrCodeNotFound,
		"ErrLinkExpired":         cairn.ErrLinkExpired,
		"ErrLinkRevoked":         cairn.ErrLinkRevoked,
		"ErrInvalidCode":         cairn.ErrInvalidCode,
		"ErrCodeSpaceExhausted":  cairn.ErrCodeSpaceExhausted,
		"ErrStoreUnavailable":    cairn.ErrStoreUnavailable,
		"ErrDestinationRejected": cairn.ErrDestinationRejected,
		"ErrVanityReserved":      cairn.ErrVanityReserved,
		"ErrVanityLength":        cairn.ErrVanityLength,
	}

	for name, sentinel := range sentinels {
		t.Run(name, func(t *testing.T) {
			wrapped := fmt.Errorf("context: %w", sentinel)
			if !errors.Is(wrapped, sentinel) {
				t.Fatalf("errors.Is(wrapped, %s) = false, want true", name)
			}
			// Sentinels must be distinguishable from one another.
			for otherName, other := range sentinels {
				if otherName == name {
					continue
				}
				if errors.Is(sentinel, other) {
					t.Fatalf("%s is indistinguishable from %s", name, otherName)
				}
			}
		})
	}
}

func TestErrDestinationRejected_WrapsReasonAndStaysComparable(t *testing.T) {
	err := cairn.NewRejectionError(cairn.ReasonScheme)

	if !errors.Is(err, cairn.ErrDestinationRejected) {
		t.Fatalf("errors.Is(err, ErrDestinationRejected) = false, want true")
	}

	reason, ok := cairn.RejectReasonFrom(err)
	if !ok {
		t.Fatalf("RejectReasonFrom(err) ok = false, want true")
	}
	if reason != cairn.ReasonScheme {
		t.Fatalf("RejectReasonFrom(err) = %q, want %q", reason, cairn.ReasonScheme)
	}
}

func TestErrDestinationRejected_SurvivesFurtherWrapping(t *testing.T) {
	err := fmt.Errorf("create: %w", cairn.NewRejectionError(cairn.ReasonUserinfo))

	if !errors.Is(err, cairn.ErrDestinationRejected) {
		t.Fatalf("errors.Is(err, ErrDestinationRejected) = false, want true after further wrapping")
	}

	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonUserinfo {
		t.Fatalf("RejectReasonFrom(err) = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonUserinfo)
	}
}

func TestRejectReasonFrom_NotARejectionError(t *testing.T) {
	_, ok := cairn.RejectReasonFrom(cairn.ErrCodeNotFound)
	if ok {
		t.Fatalf("RejectReasonFrom(ErrCodeNotFound) ok = true, want false")
	}
}

// RejectReason values are a stable, exported string enum (IR-04): every value
// must be a non-empty snake_case token, since a host may serialize it directly
// into an API response.
func TestRejectReason_Values(t *testing.T) {
	tests := []struct {
		got  cairn.RejectReason
		want string
	}{
		{cairn.ReasonScheme, "scheme_not_allowed"},
		{cairn.ReasonUserinfo, "credentials_in_url"},
		{cairn.ReasonPrivateAddress, "private_or_internal_host"},
		{cairn.ReasonTooLong, "destination_too_long"},
		{cairn.ReasonOwnDomain, "own_domain"},
		{cairn.ReasonControlChars, "control_characters"},
		{cairn.ReasonResolveFailed, "host_resolution_failed"},
		{cairn.ReasonNotAllowlisted, "outside_allowlist"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
