// Package cairn is a URL shortener library in which security is a requirement
// rather than an addendum.
//
// # Status
//
// Pre-implementation. This package declares nothing yet: the requirements,
// threat model, architecture and decision records are the deliverable of the
// current phase, and implementation follows the milestones on the project
// board.
//
// See REQUIREMENTS.md, docs/THREAT-MODEL.md, docs/ARCHITECTURE.md and
// docs/adr/ in the repository.
//
// # Scope
//
// This package owns short-code generation and resolution, destination policy,
// storage abstraction, and link lifecycle. It does not own HTTP routing,
// authentication, authorization, rate limiting, or log transport, and it does
// not import net/http.
package cairn
