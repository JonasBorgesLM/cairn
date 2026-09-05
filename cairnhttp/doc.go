// Package cairnhttp provides an optional HTTP redirect handler for cairn.
//
// It exists so that the core package does not have to import net/http. Using
// it is optional; the redirect semantics it implements are not optional for
// anyone who writes their own handler instead, and they are documented here so
// that such a handler can be written correctly:
//
//   - 302, never 301. A cached permanent redirect outlives revocation in every
//     intermediary holding it (SR-11).
//   - Cache-Control: no-store. Choosing 302 is only half of it — RFC 9111
//     section 4.2.2 permits heuristic caching of a 302 with no explicit
//     directives (SR-12).
//   - Referrer-Policy: no-referrer, so the destination does not learn the short
//     code (SR-14).
//   - 405 for anything but GET and HEAD (SR-13).
//
// See docs/adr/0006-redirect-status-and-caching.md.
package cairnhttp
