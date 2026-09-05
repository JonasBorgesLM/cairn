# CLAUDE.md

Guidance for Claude Code when working in this repository.

The general engineering rules — effort proportional to the task, architecture
discipline, clean code, testing, review, security, git hygiene, verification —
are in `~/.claude/CLAUDE.md` and are already loaded. **This file carries only
what is true of cairn**, and where it repeats a global rule it is because this
repository makes it stricter or has paid for it specifically.

## What cairn is

A Go library for URL shortening with security as a first-class requirement:
short-code generation and resolution, destination policy, storage abstraction,
and link lifecycle. It is **not** an HTTP service, and the core does not import
`net/http`.

Requirements live in [`REQUIREMENTS.md`](REQUIREMENTS.md) and are referenced by
id (`FR-01`, `SR-07`, `NFR-02`, `IR-04`…). Threats live in
[`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md) as `T-nn`. Decisions live in
[`docs/adr/`](docs/adr/README.md).

## Current phase

**Pre-implementation.** The module skeletons, CI/CD and conventions are in
place (M0); no domain code exists yet. Work is tracked on the project board,
grouped M0 through M9. Do not write implementation code without an issue that
says to.

## Repository layout

**Multi-module** (ADR-0001). Two published modules.

| Path | Module | Notes |
| --- | --- | --- |
| `.` | core | Only `github.com/JonasBorgesLM/moat` in `require` — CI enforces it |
| `redisstore/` | Redis store | go-redis; testcontainers (test only) |

`policy/`, `memstore/` and `cairnhttp/` are packages of the core module, not
modules.

A green build in one module says nothing about the other. Run per module:

```bash
go work init . ./redisstore     # once; go.work is deliberately not committed
for m in . redisstore; do (cd "$m" && go build ./... && go vet ./... && go test -race ./...); done
./.github/scripts/check-docs.sh
```

`go.work` is not committed on purpose: a committed workspace would make the
`satellite-resolution` CI job — the only one that runs with `GOWORK=off`, and
the only one that can catch a stale `require` — prove nothing.

## What CI checks, and why you cannot talk it out of it

ADR-0017: a convention CI does not check is documentation, not a convention.
Before writing code, know which jobs will have an opinion:

| Job | Fails when |
| --- | --- |
| `dependency-policy` | the core gains a third-party require, imports `net/http` outside `cairnhttp`, or `moat` is not pinned to an exact release |
| `satellite-resolution` | `redisstore` requires a core version that does not satisfy it — the failure `moat`'s redisstore/v0.2.0 shipped with |
| `adr-immutability` | an existing ADR loses a line (amend or supersede; never rewrite) |
| `commits` | a commit or the PR title is not a Conventional Commit |
| `docs` | a cited `SR-`/`T-` id does not exist, a relative link is broken, or a package exporting an API has no `ExampleXxx` |
| `lint` | an exported identifier has no doc comment, among much else |
| `CI OK` | any of the above — this is the single required check |

Two escape hatches exist and both are visible on the PR: label
`cross-module-window` for a change that must touch a core API and its consumer
together, `adr-typo` for a genuine typo in an accepted ADR. Reach for them
deliberately or not at all.

Fixing the check rather than the code is not available. If a check is wrong,
that is an issue and an ADR amendment, not a `continue-on-error`.

## Graphify in this repository

The general rules are in `~/.claude/CLAUDE.md`. What is specific here:

**The graph is currently a document map, not a call graph.** cairn has five
`doc.go` files and no domain code, so `graphify update .` produces ~280 nodes
that are entirely requirements, threats, ADRs and their cross-references —
`graph_stats` reports `EXTRACTED: 100%` because there is nothing to infer.
Asking it what calls a function will correctly return nothing, and that is the
repository's state rather than a failure of the tool. It becomes a call graph
at M1.

**What it is good for today.** Requirement and threat ids are the connective
tissue of these documents, and the graph indexes them:

```bash
graphify query "SR-18"          # every document citing the conditional-write requirement
graphify query "ADR-0007"       # what depends on the moat/secret decision
graphify god-nodes              # today: the threat model, at 16 edges
```

That last one is worth noticing rather than skimming past. The most connected
node in this repository is `docs/THREAT-MODEL.md` §4, which is the intended
shape for a library whose reason to exist is the threat model — and it is a
cheap regression check. If the highest-degree node ever becomes something
incidental, the documentation has drifted from what the project claims to be
about.

**What it will be good for at M1 and after.** The boundaries CI enforces are
graph properties, so the graph answers them in a second where CI takes a
minute:

```bash
graphify affected "Destination"   # blast radius before touching the redacting type
graphify query "net/http"         # NFR-06: nothing outside cairnhttp may reach it
graphify query "redisstore"       # ADR-0001: the core must not import a submodule
```

This is a pre-check, not a substitute. `dependency-policy` in CI is the
authority, because it asks the Go resolver rather than a parsed approximation.

**Rebuild before trusting it.** `graphify update .` takes about a second here.
A graph built before your edits will answer questions about code you have
already changed, with no indication that it is doing so.

## Definition of Done

A change is not finished when it compiles. It is finished when, for every module
it touches:

1. `go build ./...`, `go vet ./...`, `go test -race ./...` pass.
2. `golangci-lint run ./...` passes.
3. Any new `SR-` guard **has been seen to fail** before it was seen to pass.
   Remove the protection, watch the test go red, put it back. An assertion nobody
   has watched go red is not verified, it is hoped.
4. The non-negotiable invariants below still hold.

Fix the underlying issue rather than working around the check. Skipping a
subtest, silencing a vet warning, or reaching for `--no-verify` do not make a
task done.

## Non-negotiable invariants

Decided, not open. Each is a defect if violated.

1. **`Save` is conditional, always** (SR-18, ADR-0008). `SetNX`, never `SET`.
   An unconditional write silently repoints a distributed link at an attacker's
   destination (T-03) — the worst outcome this library has.
2. **Validate the code before building the key** (SR-21). Alphabet and length
   checks come *before* concatenation into the keyspace, not after. The ordering
   is the requirement.
3. **Never 301, and always `no-store`** (SR-11, SR-12, ADR-0006). 302 alone is
   heuristically cacheable under RFC 9111 §4.2.2. Both, or revocation is a lie.
4. **Fail-closed** (SR-20). Store down means an error on both paths, never a
   redirect and never a success. There is no option to fail open, and adding one
   is not a feature request.
5. **The core imports no third-party package** (NFR-01, ADR-0001). `moat` is the
   single first-party exception, pinned exactly, never auto-bumped.
6. **`Destination` has no unredacted formatting path** (SR-15). If you add a
   method to it, ask what `%v` does first.
7. **The core does not import `net/http`** (NFR-06). `cairnhttp` does; that is
   what it is for.
8. **`Revoke` does not check ownership** (ADR-0010). Authorization is the host's.
   Do not add a caller identity to it "for safety" — a half-authorization inside
   a library reads like a guarantee and is not one.
9. **Retry is bounded** (SR-19). Generated codes only; vanity codes return
   `ErrCodeExists` to the caller rather than silently issuing a different code.
10. **Hooks are called synchronously and must not block** (ADR-0016). cairn does
    not spawn a goroutine per event.
11. **No global state** (NFR-04). No package-level mutable config, no `init()`
    that registers anything.

## Conventions

Git hygiene, Conventional Commits and the review discipline are global. These
are cairn's own, each tied to a requirement and enforced somewhere:

- **English** for all code, comments, documentation and commit messages
  (NFR-12), regardless of the language a request was written in.
- **Commit scopes** are this repository's packages: `cairn`, `policy`,
  `memstore`, `cairnhttp`, `redisstore`, `adr`, `docs`, `deps`, `ci`,
  `security`. The `commits` CI job rejects anything else.
- **Every structural decision gets an ADR** (NFR-14). Do not silently resolve a
  question listed as open in `docs/adr/README.md` — write the ADR first.
- **ADRs are never rewritten.** Amend in place, or supersede with a new one that
  names the old. `adr-immutability` in CI fails when an accepted ADR loses a
  line.
- **Go 1.24 is the floor** in the core (NFR-02). Do not raise it for
  convenience; a library's `go` directive is a promise about who may import it.
- **Tests are table-driven**, use `t.Run` subtests, and assert the specific
  behaviour — not just "no error".
- **Public packages carry testable examples** (`ExampleXxx`), matching `moat`
  and `crier` (NFR-09). `check-docs.sh` fails without them.
- **Integration tests use testcontainers against real Redis** (NFR-10), never
  miniredis. A fake that implements `SetNX` correctly proves nothing about the
  server that has to.
- **Lua scripts in `.lua` files** via `go:embed` (NFR-11), never Go string
  literals.
- **Branches**: feature → `develop` via PR; `main` is the release branch
  (NFR-16). Both are protected and require the `CI OK` check.

## Writing security tests

Every `SR-` needs a test with a **negative control**. The pattern:

```go
// SR-18: Save must not overwrite an existing code.
// Negative control: verified failing against a store whose Save uses SET.
```

A test that passes against a broken implementation is worse than no test,
because someone will cite it. If you cannot make a check fail on demand, say so
in the PR rather than claiming coverage.

## Things that have gone wrong in libraries of this shape

Written down so they are avoided rather than rediscovered:

- **The interstitial that is an open redirect.** `/warn?to=<url>` with a continue
  button *is* the vulnerability the interstitial was added to prevent. The
  continuation references the code, never a URL (SR-24, ADR-0014).
- **The counter that uses the request context.** It is cancelled when the
  response completes, so every count races the redirect. Use
  `context.WithoutCancel` (ADR-0012).
- **Modulo bias in code generation.** `b % 62` over a uniform byte favours the
  first eight runes. Rejection sampling (ADR-0002).
- **Header order.** `cairnhttp` must set `Cache-Control` and `Referrer-Policy`
  immediately before `WriteHeader`, or a surrounding `secureheaders` middleware
  overwrites them. Tested with the chain in place, not alone.
- **Validating after key construction.** Reversing the order in SR-21 produces
  code that looks identical and is not.

## Tooling, and where it helps here

Installed globally; this is what is worth reaching for in *this* repository.

- **`/security-review`, and `claude-security` for a deeper pass.** cairn's
  reason to exist is its threat model, so a security review here is not a
  formality — it is the product. Every finding must name the `SR-` it breaks
  and the `T-` it enables, or it is not a finding, it is a worry.
- **Superpowers' `systematic-debugging` and `writing-plans`** for M1 onward.
  Its `test-driven-development` skill fits the `SR-` work particularly well,
  because a negative control *is* red-green-refactor: watch it fail with the
  protection removed, then restore it.
- **`/impeccable` and the animation skills do not apply here.** cairn has no
  frontend and will not get one — `cairnhttp` renders one plain interstitial
  page, deliberately unstyled so a host replaces it (ADR-0014). If you find
  yourself polishing it, the boundary has been crossed.
- **Playwright does not apply either.** There is no web application. The
  equivalent for this repository is the threat-probe script of NFR-17, run
  against the demo stack.
- **Graphify** is covered in its own section above.

The general rule that effort is proportional to the task applies with one
exception, stated because this repository is where it bites: **anything
touching an `SR-` requirement is a complex task regardless of its diff size.**
A one-line change to code generation, conditional write, code validation
ordering, or the redaction surface gets the full flow, because the line count
is not what makes those dangerous.
