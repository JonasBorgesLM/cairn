# Contributing to cairn

## Status

`cairn` is pre-implementation. The current phase is requirements, threat model,
architecture and ADRs. Issues labelled `phase-0` are documentation work; the
rest are blocked until M0 closes.

## Before you write code

1. Find the requirement id (`FR-`, `SR-`, `NFR-`, `IR-`) your change implements.
   If there is not one, the change needs a requirement first.
2. If the change is structural, it needs an ADR **before** the code. See
   [`docs/adr/README.md`](docs/adr/README.md).
3. If it touches an `SR-`, plan the negative control before the test.

## Workflow

- Branch from `develop`. `main` is the release branch (NFR-16).
- Branch names: `feat/<short-name>`, `fix/<short-name>`, `docs/adr-<nnnn>`.
- [Conventional Commits](https://www.conventionalcommits.org/), scope = package:
  `feat(policy): reject IPv4-mapped IPv6 destinations`.
- **One subject per commit**, staged explicitly. `git add -A` across two subjects
  produces a commit nobody can revert cleanly.
- PRs target `develop` and reference the requirement ids they close.

## Per-module commands

This is a multi-module repository (ADR-0001). A green build in one module says
nothing about the other.

```bash
for m in . redisstore; do
  (cd "$m" && go build ./... && go vet ./... && go test -race ./...)
done
golangci-lint run ./...
govulncheck ./...
```

## Testing rules

- Table-driven, `t.Run` subtests, assert the specific behaviour.
- Every public package has `ExampleXxx` functions (NFR-09).
- Integration tests use testcontainers against a real Redis (NFR-10). Never
  miniredis: a fake that implements `SetNX` correctly proves nothing about the
  server that has to.
- **Every `SR-` test carries a negative control.** Remove the protection, watch
  the test fail, restore it. Note the control in a comment above the test. A
  check nobody has seen go red is not a check.

## Reviewing

A reviewer should ask, in this order:

1. Which requirement does this implement, and does the code match its wording?
2. Which threat does it touch, and is the residual still what the threat model
   says it is?
3. Has the `SR-` test been seen to fail?
4. Does it violate any invariant in [`CLAUDE.md`](CLAUDE.md)?
5. Does it add a dependency to the core module? (The answer is no.)

## Reporting a vulnerability

See [`SECURITY.md`](SECURITY.md). Do not open a public issue.
