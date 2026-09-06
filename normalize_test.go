package cairn_test

import (
	"net/url"
	"testing"

	"github.com/JonasBorgesLM/cairn"
)

func normalize(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return cairn.NormalizeURL(u).String()
}

// --- Applied rules (RFC 3986 §6.2.2, ADR-0015) ---

func TestNormalizeURL_LowercasesScheme(t *testing.T) {
	if got, want := normalize(t, "HTTP://example.com/"), "http://example.com/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_LowercasesHost(t *testing.T) {
	if got, want := normalize(t, "http://EXAMPLE.com/path"), "http://example.com/path"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_RemovesDefaultPort(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"http://example.com:80/", "http://example.com/"},
		{"https://example.com:443/", "https://example.com/"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := normalize(t, tt.raw); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeURL_KeepsNonDefaultPort(t *testing.T) {
	tests := []string{"http://example.com:8080/", "https://example.com:80/"}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if got := normalize(t, raw); got != raw {
				t.Fatalf("got %q, want unchanged %q", got, raw)
			}
		})
	}
}

func TestNormalizeURL_DecodesUnreservedPercentEncoding(t *testing.T) {
	// %7E is '~' and %41 is 'A', both unreserved and simply decoded. %2E is
	// '.', also unreserved, but RFC 3986 applies percent-decoding (§6.2.2.2)
	// before dot-segment removal (§6.2.2.3): once decoded it is an ordinary
	// trailing "/." dot-segment and collapses per that later rule.
	if got, want := normalize(t, "http://example.com/%7Euser/%41/%2E"), "http://example.com/~user/A/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_UppercasesHexOfRemainingEscapes(t *testing.T) {
	// %2f is a reserved character (/) and must stay escaped, not become a
	// literal path separator -- only its hex digits are normalized to upper.
	if got, want := normalize(t, "http://example.com/a%2fb"), "http://example.com/a%2Fb"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_EmptyPathBecomesSlash(t *testing.T) {
	if got, want := normalize(t, "http://example.com"), "http://example.com/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_ResolvesDotSegments(t *testing.T) {
	if got, want := normalize(t, "http://example.com/a/./b/../c"), "http://example.com/a/c"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// --- Rejected rules (ADR-0015): each of these must NOT happen ---

func TestNormalizeURL_DoesNotSortQueryParameters(t *testing.T) {
	if got, want := normalize(t, "http://example.com/?b=2&a=1"), "http://example.com/?b=2&a=1"; got != want {
		t.Fatalf("got %q, want %q (query order must survive)", got, want)
	}
}

func TestNormalizeURL_DoesNotStripTrailingSlash(t *testing.T) {
	if got, want := normalize(t, "http://example.com/docs/"), "http://example.com/docs/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_DoesNotAddTrailingSlash(t *testing.T) {
	if got, want := normalize(t, "http://example.com/docs"), "http://example.com/docs"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_DoesNotLowercasePath(t *testing.T) {
	if got, want := normalize(t, "http://example.com/DocS"), "http://example.com/DocS"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_DoesNotRemoveWWW(t *testing.T) {
	if got, want := normalize(t, "http://www.example.com/"), "http://www.example.com/"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_DoesNotDropFragment(t *testing.T) {
	if got, want := normalize(t, "http://example.com/page#section"), "http://example.com/page#section"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeURL_DoesNotRemoveTrackingParameters(t *testing.T) {
	raw := "http://example.com/?utm_source=newsletter&utm_medium=email"
	if got := normalize(t, raw); got != raw {
		t.Fatalf("got %q, want unchanged %q", got, raw)
	}
}

// A pre-signed URL's query string is order-sensitive (it is covered by a
// signature computed over the exact bytes) and must survive normalization
// byte for byte.
func TestNormalizeURL_PreservesPreSignedQueryString(t *testing.T) {
	raw := "https://s3.example.com/bucket/key" +
		"?X-Amz-Algorithm=AWS4-HMAC-SHA256" +
		"&X-Amz-Credential=AKIAEXAMPLE%2F20250101%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Date=20250101T000000Z" +
		"&X-Amz-Expires=3600" +
		"&X-Amz-Signature=abcdef0123456789"
	if got := normalize(t, raw); got != raw {
		t.Fatalf("pre-signed query string changed:\ngot  %q\nwant %q", got, raw)
	}
}
