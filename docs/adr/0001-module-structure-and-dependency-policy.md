# ADR-0001: Module structure and dependency policy

## Status
Accepted

## Context
`cairn` has to persist links, and the obvious backend is Redis. Putting
`go-redis` in the root module would put it in the dependency graph of every
consumer, including those who bring their own store — the same problem `moat`
solved by splitting `redisstore` out and `crier` solved with one module per
exporter.

There is a second question this project has that neither of those had: the core
takes a dependency on `moat` itself (ADR-0007). That makes "zero dependencies"
false as a slogan, so the policy has to be stated as something that is actually
true and actually enforceable.

## Decision
Two published modules:

| Path | Module | Dependencies |
| --- | --- | --- |
| `.` | `github.com/JonasBorgesLM/cairn` | `github.com/JonasBorgesLM/moat` only |
| `redisstore/` | `github.com/JonasBorgesLM/cairn/redisstore` | go-redis; testcontainers (test only) |

The dependency policy is: **the core module admits first-party dependencies,
pinned exactly, and no third-party ones.** CI enforces the second half by
failing on any `require` in the core `go.mod` outside the
`github.com/JonasBorgesLM/` prefix — a policy that is not checked is a
preference.

`memstore/`, `policy/` and `cairnhttp/` are packages of the core module, not
modules. They add no dependencies (`net/http`, `net/url` and `net/netip` are
standard library), so a separate module would buy nothing and cost a tag.

Go floor: **1.24** in the core, matching `moat` core and `crier` core (NFR-02).
`redisstore` declares whatever its dependencies impose, and the `go.mod` records
that as an imposition — the same wording `crier`'s NFR-02 uses — so nobody later
reads the higher floor as an endorsement.

## Consequences
- A consumer with their own `Store` never sees `go-redis`.
- Two tag prefixes to maintain: `vX.Y.Z` and `redisstore/vX.Y.Z`. A green build
  in one module says nothing about the other; every command in CI and in
  `CLAUDE.md` runs per module.
- The core's `go.mod` is not empty, which is a visible break from `moat` and
  `crier`. ADR-0007 owns that decision and its reopening criterion; this ADR
  only records the shape it forces on the policy.
- A local `go.work` is used for development and is not committed, following
  `moat`.

## Open
Whether `cairnhttp` should become its own module if it ever grows a template
dependency. Today it uses `html/template`, so the question is not live.
