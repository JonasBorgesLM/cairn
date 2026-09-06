package cairnhttp

import (
	"net/http"
	"strings"

	"github.com/JonasBorgesLM/cairn"
)

// Handler resolves a code from the request path and redirects to its
// destination. Mount it at the root of whatever it serves — it treats the
// whole request path (minus the leading slash) as the code.
type Handler struct {
	shortener    *cairn.Shortener
	errorEncoder ErrorEncoder
}

// Option configures a Handler at construction.
type Option func(*Handler)

// WithErrorEncoder overrides the default error-to-response mapping. See
// ErrorEncoder and DefaultErrorEncoder.
func WithErrorEncoder(enc ErrorEncoder) Option {
	return func(h *Handler) { h.errorEncoder = enc }
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
