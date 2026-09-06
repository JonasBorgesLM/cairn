module github.com/JonasBorgesLM/cairn

// Go 1.24 is the floor, and it is the *lowest viable* version rather than the
// newest available (NFR-02). A library's `go` directive is a compatibility
// promise about who may import it: raising it strands consumers without
// patching anything for them. It moves only when something concrete makes it
// non-viable, and that reasoning is written as an ADR before this line changes.
//
// This module has no `require` block yet, and when it gains one it may name
// only modules under github.com/JonasBorgesLM/ (ADR-0001, NFR-01). The
// `dependency-policy` job in .github/workflows/ci.yml enforces that; a policy
// that is not checked is a preference.
//
// The one first-party dependency this module will take is
// github.com/JonasBorgesLM/moat, for secret.Value (ADR-0007). It arrives with
// destination.go, pinned to an exact version, and Dependabot is configured not
// to bump it.
go 1.24

require github.com/JonasBorgesLM/moat v0.2.0
