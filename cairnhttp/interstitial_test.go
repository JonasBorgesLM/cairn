package cairnhttp_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/memstore"
)

// interstitialPolicy classifies example.com as Allow and everything else as
// Interstitial, matching how policy.Allowlist behaves (FR-13).
type interstitialPolicy struct{}

func (interstitialPolicy) Evaluate(_ context.Context, d cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	if d.Host() == "example.com" {
		return cairn.Allow, "", nil
	}
	return cairn.Interstitial, cairn.ReasonNotAllowlisted, nil
}

func newInterstitialShortener(t *testing.T) *cairn.Shortener {
	t.Helper()
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(interstitialPolicy{}))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	return s
}

func TestHandler_InterstitialLink_RedirectsNormallyWithoutWithInterstitial(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/")
	if !link.Interstitial {
		t.Fatalf("link.Interstitial = false, want true")
	}

	h := cairnhttp.NewHandler(s) // no WithInterstitial: opt-in only (ADR-0014)
	rec := doGet(h, "/"+string(link.Code))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (redirects normally without WithInterstitial)", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "https://unknown.example/" {
		t.Fatalf("Location = %q, want %q", got, "https://unknown.example/")
	}
}

func TestHandler_InterstitialLink_RendersWarningPageWhenConfigured(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/path")

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	rec := doGet(h, "/"+string(link.Code))

	if rec.Code == http.StatusFound {
		t.Fatalf("status = %d, want the warning page, not an immediate redirect", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://unknown.example/path") {
		t.Fatalf("body does not show the destination: %s", rec.Body.String())
	}
}

func TestHandler_InterstitialLink_ContinueRedirectsToTheStoredDestination(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/path")

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	rec := doGet(h, "/"+string(link.Code)+"?continue=1")

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "https://unknown.example/path" {
		t.Fatalf("Location = %q, want %q", got, "https://unknown.example/path")
	}
}

func TestHandler_AllowedLink_NeverShowsTheInterstitialEvenWhenConfigured(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://example.com/") // Allow, not Interstitial

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	rec := doGet(h, "/"+string(link.Code))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (Allow decision, never interstitial)", rec.Code, http.StatusFound)
	}
}

// SR-24, ADR-0014: the continuation control references the code only. No
// destination is ever accepted from the request -- an extra query parameter
// naming a URL must be ignored entirely, proven by asserting on the response
// rather than by review.
//
// Negative control: this test (and TestHandler_NeverReflectsAQueryParameterAsADestination
// below) was run against a build of ServeHTTP that read a "to" query
// parameter and substituted it for the stored destination when present --
// the literal open redirect ADR-0014 exists to prevent. It failed: Location
// became "https://evil.example/". Restored immediately after.
func TestHandler_ContinueIgnoresAnyDestinationSuppliedInTheRequest(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/real-destination")

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	rec := doGet(h, "/"+string(link.Code)+"?continue=1&to="+url.QueryEscape("https://evil.example/"))

	if got := rec.Header().Get("Location"); got != "https://unknown.example/real-destination" {
		t.Fatalf("Location = %q, want the stored destination, unaffected by the extra query parameter", got)
	}
}

// No handler in this package reads a URL from a query parameter -- asserted
// by a test, not by review: a request carrying a candidate destination in
// every plausible parameter name must never be reflected anywhere in the
// response.
func TestHandler_NeverReflectsAQueryParameterAsADestination(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/")
	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))

	const poison = "https://attacker.example/open-redirect"
	for _, param := range []string{"to", "url", "redirect", "destination", "dest", "next"} {
		t.Run(param, func(t *testing.T) {
			rec := doGet(h, "/"+string(link.Code)+"?"+param+"="+url.QueryEscape(poison))
			if strings.Contains(rec.Header().Get("Location"), "attacker.example") {
				t.Fatalf("Location = %q, reflects the %q parameter", rec.Header().Get("Location"), param)
			}
			if strings.Contains(rec.Body.String(), "attacker.example") {
				t.Fatalf("body reflects the %q parameter", param)
			}
		})
	}
}

// The default template loads no external resource: a warning page that pulls
// third-party script to say a third party is untrusted is a contradiction
// and a destination leak in itself (ADR-0014).
func TestDefaultInterstitialTemplate_LoadsNoExternalResource(t *testing.T) {
	s := newInterstitialShortener(t)
	link := createLink(t, s, "https://unknown.example/")
	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))

	rec := doGet(h, "/"+string(link.Code))
	body := rec.Body.String()

	// The destination itself legitimately appears as escaped text; the
	// violation this checks for is a *resource reference* (a script, style,
	// image or link tag pointing off-page), not the presence of a scheme
	// anywhere in the body.
	for _, attr := range []string{`src="http`, `href="http`, `src='http`, `href='http`} {
		if strings.Contains(body, attr) {
			t.Fatalf("body contains %q: an external resource reference", attr)
		}
	}
}

// The destination renders as escaped text: an XSS attempt in it must appear
// literally, never as markup the browser executes.
//
// Negative control: this test was run against a build using text/template
// instead of html/template for the interstitial (no contextual escaping),
// and it failed -- the payload appeared unescaped in the body. Reverted
// immediately after.
func TestDefaultInterstitialTemplate_EscapesTheDestination(t *testing.T) {
	s := newInterstitialShortener(t)
	const payload = "https://unknown.example/?x=<script>alert(1)</script>"
	link := createLink(t, s, payload)

	h := cairnhttp.NewHandler(s, cairnhttp.WithInterstitial(cairnhttp.DefaultInterstitialTemplate))
	rec := doGet(h, "/"+string(link.Code))

	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Fatalf("body contains an unescaped <script> tag: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("body does not contain the escaped payload: %s", body)
	}
}
