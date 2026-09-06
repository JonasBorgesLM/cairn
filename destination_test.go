package cairn_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

const secretToken = "s3cr3t-token-do-not-leak"

func validDestinationWithSecret(t *testing.T) cairn.Destination {
	t.Helper()
	d, err := cairn.ParseDestination("https://example.com/reset?token=" + secretToken)
	if err != nil {
		t.Fatalf("ParseDestination error = %v, want nil", err)
	}
	return d
}

// --- ParseDestination: syntax and shape checks (SR-05, SR-06, SR-08, SR-10) ---

func TestParseDestination_AcceptsValidHTTPSURL(t *testing.T) {
	d, err := cairn.ParseDestination("https://example.com/path?a=1")
	if err != nil {
		t.Fatalf("ParseDestination error = %v, want nil", err)
	}
	if d.Scheme() != "https" {
		t.Fatalf("Scheme() = %q, want %q", d.Scheme(), "https")
	}
	if d.Host() != "example.com" {
		t.Fatalf("Host() = %q, want %q", d.Host(), "example.com")
	}
	raw, err := d.URL()
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	if raw.String() != "https://example.com/path?a=1" {
		t.Fatalf("URL().String() = %q, want %q", raw.String(), "https://example.com/path?a=1")
	}
}

// SR-08: length checked before parsing, default max 2000 bytes.
//
// Negative control: this test was run against a build of ParseDestination
// with the length check removed. It failed -- error was nil. Restored
// immediately after.
func TestParseDestination_RejectsTooLong(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 2000)
	_, err := cairn.ParseDestination(long)
	if err == nil {
		t.Fatalf("ParseDestination(2000+ path) error = nil, want non-nil")
	}
	if !errors.Is(err, cairn.ErrDestinationRejected) {
		t.Fatalf("error = %v, want errors.Is(_, ErrDestinationRejected)", err)
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonTooLong {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonTooLong)
	}
}

func TestParseDestination_AcceptsAtLengthBoundary(t *testing.T) {
	// Exactly 2000 bytes must be accepted; the limit rejects anything over it.
	padding := strings.Repeat("a", 2000-len("https://example.com/"))
	_, err := cairn.ParseDestination("https://example.com/" + padding)
	if err != nil {
		t.Fatalf("ParseDestination at exactly 2000 bytes error = %v, want nil", err)
	}
}

// SR-05: scheme allowlist, never a denylist.
//
// Negative control: verified failing (accepting javascript:) when the scheme
// check was temporarily removed from ParseDestination during development.
func TestParseDestination_RejectsSchemesOutsideAllowlist(t *testing.T) {
	tests := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"file:///etc/passwd",
		"ftp://example.com/file",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := cairn.ParseDestination(raw)
			if err == nil {
				t.Fatalf("ParseDestination(%q) error = nil, want non-nil", raw)
			}
			reason, ok := cairn.RejectReasonFrom(err)
			if !ok || reason != cairn.ReasonScheme {
				t.Fatalf("RejectReasonFrom(%q) = (%q, %v), want (%q, true)", raw, reason, ok, cairn.ReasonScheme)
			}
		})
	}
}

func TestParseDestination_AcceptsHTTPAndHTTPSSchemesCaseInsensitively(t *testing.T) {
	for _, raw := range []string{"http://example.com/", "https://example.com/", "HTTP://example.com/", "HTTPS://example.com/"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := cairn.ParseDestination(raw); err != nil {
				t.Fatalf("ParseDestination(%q) error = %v, want nil", raw, err)
			}
		})
	}
}

// SR-06: no embedded credentials.
//
// Negative control: verified failing (accepting the credentials) when the
// userinfo check was temporarily removed from ParseDestination during
// development.
func TestParseDestination_RejectsUserinfo(t *testing.T) {
	_, err := cairn.ParseDestination("https://accounts.example.com@evil.example/")
	if err == nil {
		t.Fatalf("ParseDestination error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonUserinfo {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonUserinfo)
	}
}

func TestParseDestination_RejectsDoubleAtUserinfo(t *testing.T) {
	_, err := cairn.ParseDestination("https://a@b@c/")
	if err == nil {
		t.Fatalf("ParseDestination(\"https://a@b@c/\") error = nil, want non-nil")
	}
}

// SR-10: no control characters or whitespace anywhere in the destination.
//
// Negative control: this test was run against a build of ParseDestination
// with the raw-level containsControlOrWhitespace check removed. It failed --
// RejectReasonFrom reported (\"\", false) instead of ReasonControlChars.
// Restored immediately after.
func TestParseDestination_RejectsCRLFInjection(t *testing.T) {
	_, err := cairn.ParseDestination("https://example.com/\r\nSet-Cookie:%20evil=1")
	if err == nil {
		t.Fatalf("ParseDestination error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonControlChars {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonControlChars)
	}
}

func TestParseDestination_RejectsNULByte(t *testing.T) {
	_, err := cairn.ParseDestination("https://example.com/\x00")
	if err == nil {
		t.Fatalf("ParseDestination error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonControlChars {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonControlChars)
	}
}

// A percent-encoded newline hidden in the host must be rejected too: the raw
// string carries no literal control byte, only the request that the host be
// interpreted with one once decoded.
func TestParseDestination_RejectsPercentEncodedNewlineInHost(t *testing.T) {
	_, err := cairn.ParseDestination("https://example.com%0d%0aevil.example/")
	if err == nil {
		t.Fatalf("ParseDestination error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonControlChars {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonControlChars)
	}
}

// ADR-0015: v1 rejects non-ASCII hostnames rather than converting them, and
// reuses ReasonControlChars to report it.
func TestParseDestination_RejectsNonASCIIHost(t *testing.T) {
	_, err := cairn.ParseDestination("https://exämple.com/")
	if err == nil {
		t.Fatalf("ParseDestination error = nil, want non-nil")
	}
	reason, ok := cairn.RejectReasonFrom(err)
	if !ok || reason != cairn.ReasonControlChars {
		t.Fatalf("RejectReasonFrom = (%q, %v), want (%q, true)", reason, ok, cairn.ReasonControlChars)
	}
}

func TestParseDestination_RejectsEmptyString(t *testing.T) {
	if _, err := cairn.ParseDestination(""); err == nil {
		t.Fatalf("ParseDestination(\"\") error = nil, want non-nil")
	}
}

// --- Destination: redaction (SR-15) ---

func TestDestination_IsZero(t *testing.T) {
	var d cairn.Destination
	if !d.IsZero() {
		t.Fatalf("zero Destination IsZero() = false, want true")
	}

	d = validDestinationWithSecret(t)
	if d.IsZero() {
		t.Fatalf("parsed Destination IsZero() = true, want false")
	}
}

func TestDestination_Equal(t *testing.T) {
	a, err := cairn.ParseDestination("https://example.com/x")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	b, err := cairn.ParseDestination("https://example.com/x")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}
	c, err := cairn.ParseDestination("https://example.com/y")
	if err != nil {
		t.Fatalf("ParseDestination error = %v", err)
	}

	if !a.Equal(b) {
		t.Fatalf("Equal(same URL) = false, want true")
	}
	if a.Equal(c) {
		t.Fatalf("Equal(different URL) = true, want false")
	}
}

func TestDestination_ExplicitUnmaskMethodsReturnTheRealValue(t *testing.T) {
	d := validDestinationWithSecret(t)

	raw := d.Raw()
	if !strings.Contains(raw, secretToken) {
		t.Fatalf("Raw() = %q, want it to contain the secret token", raw)
	}

	u, err := d.URL()
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	if !strings.Contains(u.String(), secretToken) {
		t.Fatalf("URL().String() = %q, want it to contain the secret token", u.String())
	}
}

// This is the central security test for SR-15: every exported formatting,
// logging and marshalling path must redact, and none may emit the secret.
//
// Negative control: this test was seen to fail — the token appeared in the
// %v and %+v output — when Destination.Format was temporarily made to write
// d.Raw() instead of the redacted form during development. It was restored
// immediately after.
func TestDestination_RedactsThroughEveryFormattingPath(t *testing.T) {
	d := validDestinationWithSecret(t)
	const wantRedacted = "https://example.com/[REDACTED]"

	//nolint:gocritic // deliberately going through fmt, not d.String(): the point of this test is that fmt's dispatch itself redacts.
	checks := map[string]string{
		"%v":     fmt.Sprintf("%v", d),
		"%s":     fmt.Sprintf("%s", d),
		"%q":     fmt.Sprintf("%q", d),
		"%+v":    fmt.Sprintf("%+v", d),
		"#v":     fmt.Sprintf("%#v", d),
		"String": d.String(),
	}

	for verb, got := range checks {
		t.Run(verb, func(t *testing.T) {
			if strings.Contains(got, secretToken) {
				t.Fatalf("%s output %q leaks the secret token", verb, got)
			}
		})
	}

	if got := d.String(); got != wantRedacted {
		t.Fatalf("String() = %q, want %q", got, wantRedacted)
	}
	//nolint:gocritic // deliberately going through fmt, not d.String(): the point of this assertion is that %v itself redacts.
	if got := fmt.Sprintf("%v", d); got != wantRedacted {
		t.Fatalf("%%v = %q, want %q", got, wantRedacted)
	}

	textBytes, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error = %v", err)
	}
	if strings.Contains(string(textBytes), secretToken) {
		t.Fatalf("MarshalText() = %q, leaks the secret token", textBytes)
	}
	if string(textBytes) != wantRedacted {
		t.Fatalf("MarshalText() = %q, want %q", textBytes, wantRedacted)
	}

	jsonBytes, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	if strings.Contains(string(jsonBytes), secretToken) {
		t.Fatalf("json.Marshal(d) = %s, leaks the secret token", jsonBytes)
	}

	logValue := d.LogValue()
	if strings.Contains(logValue.String(), secretToken) {
		t.Fatalf("LogValue().String() = %q, leaks the secret token", logValue.String())
	}
}

// A Destination nested as an exported field of another struct must still
// redact under both a slog TextHandler and a slog JSONHandler — the case
// SR-15 exists for, since a leak is usually an incidental %+v or slog.Any on
// a containing struct, never a deliberate print of the Destination alone.
func TestDestination_RedactsWhenNestedInAStructLoggedViaSlog(t *testing.T) {
	type event struct {
		Dest cairn.Destination
	}
	ev := event{Dest: validDestinationWithSecret(t)}

	if strings.Contains(fmt.Sprintf("%+v", ev), secretToken) {
		t.Fatalf("%%+v on containing struct leaks the secret token: %+v", ev)
	}

	var textBuf bytes.Buffer
	slog.New(slog.NewTextHandler(&textBuf, nil)).Info("event", slog.Any("event", ev))
	if strings.Contains(textBuf.String(), secretToken) {
		t.Fatalf("slog TextHandler output leaks the secret token: %s", textBuf.String())
	}

	var jsonBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&jsonBuf, nil)).Info("event", slog.Any("event", ev))
	if strings.Contains(jsonBuf.String(), secretToken) {
		t.Fatalf("slog JSONHandler output leaks the secret token: %s", jsonBuf.String())
	}
}

func TestDestination_UnmarshalText_RefusesRedactedPlaceholder(t *testing.T) {
	d := validDestinationWithSecret(t)
	redacted, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error = %v", err)
	}

	var round cairn.Destination
	err = round.UnmarshalText(redacted)
	if err == nil {
		t.Fatalf("UnmarshalText(%q) error = nil, want non-nil", redacted)
	}
}

func TestDestination_UnmarshalJSON_RefusesRedactedPlaceholder(t *testing.T) {
	d := validDestinationWithSecret(t)
	redacted, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}

	var round cairn.Destination
	err = json.Unmarshal(redacted, &round)
	if err == nil {
		t.Fatalf("json.Unmarshal(%s) error = nil, want non-nil", redacted)
	}
}

func TestDestination_UnmarshalText_AcceptsAGenuineURL(t *testing.T) {
	var d cairn.Destination
	if err := d.UnmarshalText([]byte("https://example.com/path")); err != nil {
		t.Fatalf("UnmarshalText error = %v, want nil", err)
	}
	if d.Host() != "example.com" {
		t.Fatalf("Host() = %q, want %q", d.Host(), "example.com")
	}
}

func ExampleDestination() {
	d, err := cairn.ParseDestination("https://example.com/reset?token=abc123")
	if err != nil {
		panic(err)
	}

	fmt.Println(d)

	// Output: https://example.com/[REDACTED]
}
