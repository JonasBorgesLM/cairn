package cairnhttp

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/JonasBorgesLM/cairn"
)

// Handler resolves a code from the request path and redirects to its
// destination. Mount it at the root of whatever it serves — it treats the
// whole request path (minus the leading slash) as the code.
type Handler struct {
	shortener        *cairn.Shortener
	errorEncoder     ErrorEncoder
	interstitialTmpl *template.Template
}

// Option configures a Handler at construction.
type Option func(*Handler)

// WithErrorEncoder overrides the default error-to-response mapping. See
// ErrorEncoder and DefaultErrorEncoder.
func WithErrorEncoder(enc ErrorEncoder) Option {
	return func(h *Handler) { h.errorEncoder = enc }
}

// WithInterstitial enables the warning page for a link Policy classified as
// Interstitial at creation, rendered with t (DefaultInterstitialTemplate, or
// a host's own). Without this option, an Interstitial link redirects
// normally: the feature is opt-in at the transport, and the classification
// stays visible on Link.Interstitial for a host rendering its own page some
// other way (ADR-0014).
func WithInterstitial(t *template.Template) Option {
	return func(h *Handler) { h.interstitialTmpl = t }
}

// NewHandler returns a Handler resolving codes through s.
func NewHandler(s *cairn.Shortener, opts ...Option) *Handler {
	h := &Handler{shortener: s, errorEncoder: DefaultErrorEncoder}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := cairn.Code(strings.TrimPrefix(r.URL.Path, "/"))
	link, err := h.shortener.Resolve(r.Context(), code)
	if err != nil {
		h.errorEncoder(w, r, err)
		return
	}

	if link.Interstitial && h.interstitialTmpl != nil && r.URL.Query().Get("continue") != "1" {
		h.renderInterstitial(w, link)
		return
	}

	dest, err := link.Dest.URL()
	if err != nil {
		h.errorEncoder(w, r, err)
		return
	}

	// Set immediately before WriteHeader, not earlier: a surrounding
	// secureheaders-shaped middleware writes to the same header map, and
	// last-writer-wins depends on middleware order. Setting these two here,
	// as the last thing this handler does, is what makes them survive
	// regardless of that order (IR-01, docs/INTEGRATION.md §2.2).
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Location", dest.String())
	w.WriteHeader(http.StatusFound)
}

// renderInterstitial writes the warning page. It carries the same no-store
// and no-referrer headers as the redirect (ADR-0014), set for the same
// reason: immediately before WriteHeader, so a surrounding middleware
// cannot overwrite them.
//
// The destination shown is read from link, which came from Store.Load —
// never from the request. This is the whole point of SR-24: there is no
// parameter anywhere in this handler that names a destination for this
// method to have read in the first place.
func (h *Handler) renderInterstitial(w http.ResponseWriter, link *cairn.Link) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	// #nosec G104 -- the status is already written; a write error here means the connection is gone, and there is nothing left to report it to.
	//nolint:errcheck // see the #nosec comment above.
	h.interstitialTmpl.Execute(w, interstitialData{Destination: link.Dest.Raw()})
}
