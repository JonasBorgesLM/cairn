package cairnhttp

import (
	"errors"
	"net/http"

	"github.com/JonasBorgesLM/cairn"
)

// ErrorEncoder writes an HTTP response for err, encountered while resolving
// a code. It is the seam a host uses to keep its own JSON error envelope
// consistent across its whole API rather than inheriting cairnhttp's plain
// text one (FR-12, IR-04).
//
// err is never nil: ErrorEncoder is called only when Resolve failed.
type ErrorEncoder func(w http.ResponseWriter, r *http.Request, err error)

// DefaultErrorEncoder maps the errors Resolve can return to plain-text HTTP
// responses, per docs/INTEGRATION.md §4.2's redirect-path subset:
//
//   - ErrInvalidCode, ErrCodeNotFound, ErrLinkExpired, ErrLinkRevoked -> 404
//   - ErrStoreUnavailable                                             -> 503, never a redirect (SR-20)
//   - anything else                                                   -> 500
//
// A resolve miss must be indistinguishable, at the transport level, from a
// resolve of a code that is revoked or expired, unless the host explicitly
// opts into distinguishing them (SR-03): distinguishing them by default helps
// a legitimate user, but it is also an oracle a scanner gets for free without
// any authentication. Collapsing all four to the same 404 here is what makes
// that distinction something a host must opt into -- by supplying its own
// ErrorEncoder via WithErrorEncoder, as docs/INTEGRATION.md's task-api example
// does, conditioned on the request being authenticated.
func DefaultErrorEncoder(w http.ResponseWriter, _ *http.Request, err error) {
	switch {
	case errors.Is(err, cairn.ErrInvalidCode), errors.Is(err, cairn.ErrCodeNotFound),
		errors.Is(err, cairn.ErrLinkExpired), errors.Is(err, cairn.ErrLinkRevoked):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, cairn.ErrStoreUnavailable):
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
