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
//   - ErrInvalidCode, ErrCodeNotFound  -> 404
//   - ErrLinkExpired, ErrLinkRevoked   -> 410
//   - ErrStoreUnavailable              -> 503, never a redirect (SR-20)
//   - anything else                    -> 500
//
// The 410-versus-404 choice for expired links belongs to the host, not to
// cairn (SR-03): distinguishing expired from not-found helps a legitimate
// user and is an oracle to a scanner. A host that wants to collapse the two
// supplies its own ErrorEncoder via WithErrorEncoder.
func DefaultErrorEncoder(w http.ResponseWriter, _ *http.Request, err error) {
	switch {
	case errors.Is(err, cairn.ErrInvalidCode), errors.Is(err, cairn.ErrCodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, cairn.ErrLinkExpired), errors.Is(err, cairn.ErrLinkRevoked):
		http.Error(w, "gone", http.StatusGone)
	case errors.Is(err, cairn.ErrStoreUnavailable):
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
