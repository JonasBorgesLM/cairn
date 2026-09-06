package cairn

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode"

	"github.com/JonasBorgesLM/moat/secret"
)

// maxDestinationLength is the conservative default maximum destination
// length, enforced before parsing (SR-08).
const maxDestinationLength = 2000

// allowedSchemes is the scheme allowlist (SR-05). It is deliberately an
// allowlist, never a denylist: a denylist is a list of things someone
// remembered to forbid.
var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

// Destination is a validated http/https URL that redacts itself through every
// formatting, logging and marshalling path the standard library offers
// (SR-15). The raw URL is held in a [secret.Value]; scheme and host are held
// in the clear because those are what an operator needs to triage an
// incident, and neither carries the secret (ADR-0007).
//
// The zero Destination is valid and reports IsZero() true.
//
// The raw form comes out only through [Destination.URL] and
// [Destination.Raw], which are explicit and visible in review.
type Destination struct {
	raw    secret.Value
	scheme string
	host   string
}

// ParseDestination checks syntax and shape only — scheme allowlist, userinfo,
// control characters, length (SR-05, SR-06, SR-08, SR-10). It performs no
// name resolution and applies no [Policy]; that is Policy's job, and it needs
// a context.
//
// ParseDestination does not normalize its input; the destination is stored
// exactly as validated. A caller that needs the RFC 3986 §6.2.2 equivalences
// applied (FR-16, ADR-0015) does so before constructing the [Link] that holds
// this Destination.
func ParseDestination(raw string) (Destination, error) {
	if len(raw) > maxDestinationLength {
		return Destination{}, NewRejectionError(ReasonTooLong)
	}
	if containsControlOrWhitespace(raw) {
		return Destination{}, NewRejectionError(ReasonControlChars)
	}

	u, err := url.Parse(raw)
	if err != nil {
		var escErr url.EscapeError
		if errors.As(err, &escErr) {
			// Go's parser refuses any percent-escape in the authority
			// component outright, valid or not — which is exactly the
			// mechanism a percent-encoded control character in the host
			// (e.g. "%0d%0a") would otherwise hide behind (SR-10).
			return Destination{}, NewRejectionError(ReasonControlChars)
		}
		return Destination{}, fmt.Errorf("%w: %w", ErrDestinationRejected, err)
	}

	if !allowedSchemes[strings.ToLower(u.Scheme)] {
		return Destination{}, NewRejectionError(ReasonScheme)
	}
	if u.User != nil {
		return Destination{}, NewRejectionError(ReasonUserinfo)
	}

	host := u.Hostname()
	if !isASCII(host) {
		// ADR-0015: v1 rejects non-ASCII hostnames rather than converting
		// them to punycode, and reuses ReasonControlChars to report it.
		return Destination{}, NewRejectionError(ReasonControlChars)
	}
	decodedHost, err := url.QueryUnescape(host)
	if err != nil {
		return Destination{}, NewRejectionError(ReasonControlChars)
	}
	if containsControlOrWhitespace(decodedHost) {
		// A percent-encoded control character in the host carries no literal
		// control byte in raw, so it survives the check above undetected
		// until decoded here.
		return Destination{}, NewRejectionError(ReasonControlChars)
	}

	return Destination{
		raw:    secret.New([]byte(raw)),
		scheme: u.Scheme,
		host:   host,
	}, nil
}

func containsControlOrWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || r == ' ' {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// URL returns the parsed destination URL. This is an explicit unmask: the
// call site is visible in review (SR-15).
func (d Destination) URL() (*url.URL, error) {
	if d.IsZero() {
		return nil, fmt.Errorf("cairn: Destination is zero")
	}
	return url.Parse(string(d.raw.Bytes()))
}

// Raw returns the raw destination string. This is an explicit unmask: the
// call site is visible in review (SR-15).
func (d Destination) Raw() string {
	if d.IsZero() {
		return ""
	}
	return string(d.raw.Bytes())
}

// Scheme returns the destination's scheme. It is safe to log.
func (d Destination) Scheme() string {
	return d.scheme
}

// Host returns the destination's host, without port. It is safe to log.
func (d Destination) Host() string {
	return d.host
}

// IsZero reports whether d holds no destination.
func (d Destination) IsZero() bool {
	return d.raw.IsZero()
}

// Equal reports whether d and other hold the same raw destination.
func (d Destination) Equal(other Destination) bool {
	return d.raw.Equal(other.raw)
}

// redacted returns the redacted rendering: scheme and host kept, everything
// after the authority replaced. It is what every formatting, logging and
// marshalling method below writes.
func (d Destination) redacted() string {
	if d.IsZero() {
		return ""
	}
	return d.scheme + "://" + d.host + "/" + secret.Redacted
}

// String implements fmt.Stringer, returning the redacted rendering.
func (d Destination) String() string {
	return d.redacted()
}

// GoString implements fmt.GoStringer, returning a Go-syntax representation
// with the destination redacted. It is what %#v prints.
func (d Destination) GoString() string {
	return `cairn.Destination("` + d.redacted() + `")`
}

// Format implements fmt.Formatter, which is what makes redaction total: fmt
// consults Formatter for every verb, including %d and %x, which Stringer
// alone would not intercept.
func (d Destination) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		//nolint:errcheck // fmt.Formatter has no error return; fmt records write errors on f itself.
		fmt.Fprint(f, d.GoString())
		return
	}
	//nolint:errcheck // fmt.Formatter has no error return; fmt records write errors on f itself.
	fmt.Fprint(f, d.redacted())
}

// LogValue implements slog.LogValuer, so a Destination logged through
// log/slog is redacted under every handler, including slog.Any and nested
// groups.
func (d Destination) LogValue() slog.Value {
	return slog.StringValue(d.redacted())
}

// MarshalText implements encoding.TextMarshaler, writing the redacted
// rendering.
func (d Destination) MarshalText() ([]byte, error) {
	return []byte(d.redacted()), nil
}

// MarshalJSON implements json.Marshaler, writing the redacted rendering as a
// JSON string.
func (d Destination) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.redacted())
}

// UnmarshalText implements encoding.TextUnmarshaler.
//
// Marshalling is lossy on purpose — it always writes the redacted form — so
// this is not its inverse. Text containing the redaction placeholder refuses
// with an error mirroring [secret.ErrRedacted], so a round trip through JSON
// cannot silently turn a redacted rendering back into a live destination.
// Any other text is parsed as a raw destination, exactly as [ParseDestination]
// would.
func (d *Destination) UnmarshalText(text []byte) error {
	if strings.Contains(string(text), secret.Redacted) {
		return secret.ErrRedacted
	}
	parsed, err := ParseDestination(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// UnmarshalJSON implements json.Unmarshaler, taking a JSON string through the
// same rules as [Destination.UnmarshalText].
func (d *Destination) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("cairn: destination must be a JSON string: %w", err)
	}
	return d.UnmarshalText([]byte(s))
}
