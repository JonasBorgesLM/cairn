package cairnhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/memstore"
)

type allowPolicy struct{}

func (allowPolicy) Evaluate(context.Context, cairn.Destination) (cairn.Decision, cairn.RejectReason, error) {
	return cairn.Allow, "", nil
}

func newTestShortener(t *testing.T) *cairn.Shortener {
	t.Helper()
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}))
	if err != nil {
		t.Fatalf("cairn.New error = %v", err)
	}
	return s
}

func createLink(t *testing.T, s *cairn.Shortener, rawURL string) *cairn.Link {
	t.Helper()
	link, err := s.Create(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	return link
}

func doGet(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandler_RedirectsWithFoundStatus(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	rec := doGet(h, "/"+string(link.Code))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (302 Found)", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "https://example.com/docs" {
		t.Fatalf("Location = %q, want %q", got, "https://example.com/docs")
	}
}

func TestHandler_SetsCacheControlNoStore(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	rec := doGet(h, "/"+string(link.Code))
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
}

func TestHandler_SetsReferrerPolicyNoReferrer(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	rec := doGet(h, "/"+string(link.Code))
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want %q", got, "no-referrer")
	}
}

func TestHandler_SetsPragmaNoCache(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	rec := doGet(h, "/"+string(link.Code))
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want %q", got, "no-cache")
	}
}

// Negative control: this test was run with the Cache-Control: no-store line
// removed from the handler, and it failed -- the header was simply absent.
// Restored immediately after.
func TestHandler_CacheControlSurvivesWithoutExplicitNoStore(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	rec := doGet(h, "/"+string(link.Code))
	cc := rec.Header().Values("Cache-Control")
	if len(cc) != 1 || cc[0] != "no-store" {
		t.Fatalf("Cache-Control = %v, want exactly [\"no-store\"]", cc)
	}
}

// No code path in this package may emit 301: an option to do so would be a
// trap, not a feature (ADR-0006). Checking for the named constant, not the
// bare digits, since the package's own doc comments correctly mention "301"
// in prose explaining why it is avoided.
func TestPackage_NeverReferencesStatusMovedPermanently(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob error = %v", err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue // this file's own text necessarily names the constant.
		}
		// #nosec G304 -- name comes from filepath.Glob("*.go") in this test's own directory, not from any external input.
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.Contains(string(content), "StatusMovedPermanently") {
			t.Fatalf("%s references http.StatusMovedPermanently (301); no code path here may emit it", name)
		}
	}
}

func TestHandler_RejectsNonGetHeadMethods(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), method, "/"+string(link.Code), http.NoBody)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			allow := rec.Header().Get("Allow")
			if !strings.Contains(allow, "GET") || !strings.Contains(allow, "HEAD") {
				t.Fatalf("Allow = %q, want it to name GET and HEAD", allow)
			}
		})
	}
}

func TestHandler_HeadRedirectsWithNoBody(t *testing.T) {
	s := newTestShortener(t)
	link := createLink(t, s, "https://example.com/docs")
	h := cairnhttp.NewHandler(s)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodHead, "/"+string(link.Code), http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD response body = %q, want empty", rec.Body.String())
	}
}
