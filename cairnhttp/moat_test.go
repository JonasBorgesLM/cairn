package cairnhttp_test

import (
	"net/http"
	"testing"

	"github.com/JonasBorgesLM/moat/secureheaders"

	"github.com/JonasBorgesLM/cairn/cairnhttp"
)

// This is the test that would otherwise never get written, because each
// component is correct alone: secureheaders sets Referrer-Policy on every
// response it wraps, and cairnhttp sets its own Referrer-Policy and
// Cache-Control on the redirect. Both write the same header map, and
// last-writer-wins depends on which one runs closer to WriteHeader. Wrapping
// the real moat middleware around the real Handler is what actually proves
// cairnhttp's headers survive, rather than assuming it from reading each in
// isolation (IR-01, docs/INTEGRATION.md §2.2).
func TestHandler_HeadersSurviveUnderASecureheadersChain(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")

	h := cairnhttp.NewHandler(s)
	wrapped := secureheaders.Middleware()(h)

	rec := doGet(wrapped, "/"+string(link.Code))

	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want %q (secureheaders' own %q must not win)",
			got, "no-referrer", secureheaders.DefaultReferrerPolicy)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
}
