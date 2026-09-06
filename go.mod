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

// v0.1.0 shipped no GitHub Release: release.yml's own module-coverage check
// (correctly) refused to publish it, because the check did not yet know
// about the demo/ module added in M8. The tag is real, signed, and already
// cached by the module proxy -- the code at it is fine -- but per
// RELEASING.md's "If a release is wrong" section, a tag is never deleted or
// moved once fetched, so this retracts it rather than reusing or discarding
// it. v0.1.1 is the first release the workflow actually completed.
retract v0.1.0 // release.yml failed closed before publishing; no GitHub Release exists for this tag
