# ADR-0017: Conventions are enforced mechanically, or they are not conventions

## Status
Accepted

## Context
This repository's `CONTRIBUTING.md` and `CLAUDE.md` assert a number of
properties: the core takes no third-party dependency, ADRs are never rewritten,
commits follow a convention, cited requirement ids resolve, every public package
carries an example, `Save` is conditional.

Every one of those is currently true because it was written down recently by
someone who meant it. That is the weakest form of true a property can have. The
evidence is in the neighbouring repositories, where the same conventions were
also written down:

- `moat` shipped `redisstore/v0.2.0` requiring a core whose interface it did not
  satisfy. Every local signal was green, and necessarily so — inside a
  workspace, the `require` line is inert.
- `crier` recorded six checks that had passed without verifying anything,
  because a negative assertion is satisfied by a tool that never ran.
- `crier`'s `main` held nothing but an initial commit through six milestones,
  because the branch model said what `main` was for and nothing said when to
  update it.
- `usher` records `moat` taking a breaking rename in a *minor* release, and a
  published satellite tag that did not compile.

None of these was a lapse of discipline. Each was a property that only a human
was checking, in a repository where the humans were being careful.

## Decision
**A convention that CI does not check is documentation, not a convention**, and
it is written down as an aspiration rather than a rule.

Concretely, before implementation begins:

| Claim | Enforced by |
| --- | --- |
| Core takes no third-party dependency (NFR-01) | `dependency-policy` job — `go list -m all` filtered by prefix |
| `moat` is pinned to an exact release (ADR-0007) | same job — a pseudo-version or branch ref fails |
| The core does not import `net/http` (NFR-06) | same job — `go list -deps` per package, `cairnhttp` excepted |
| A satellite's `require` is real (ADR-0001) | `satellite-resolution` job — the only one with `GOWORK=off` |
| ADRs are amended, never rewritten (NFR-14) | `adr-immutability` job — an existing ADR may gain lines, never lose them |
| Conventional Commits (NFR-13) | `commits` job — every commit **and** the PR title, since a squash uses the title |
| Cited requirement and threat ids exist | `check-docs.sh` — bidirectional against REQUIREMENTS.md and THREAT-MODEL.md |
| Relative document links resolve | `check-docs.sh` |
| Public packages carry examples (NFR-09) | `check-docs.sh` — skipped while a package exports nothing |
| Exported identifiers are documented | `revive`'s `exported` rule |
| Tools are reproducible | every version pinned exactly; no `@latest` anywhere |

Three properties of how these are written matter as much as that they exist:

**They fail closed.** An empty `.github/allowed_signers` blocks every release
rather than warning. `crier` learned this one specifically: its signature check
warned and continued either way, merging "not signed" with "CI cannot check"
into one message that gated nothing.

**They can be run locally.** `check-docs.sh` is a file, not inline YAML. A check
people meet for the first time on a red pull request is a check they learn to
resent, then to skip.

**They explain the fix, not just the verdict.** Each failure prints what to do
and why the rule exists. A checker that only says no gets disabled.

### What is deliberately not enforced

- **That the body of a commit explains why.** Unmeasurable, and a regex would
  reward padding.
- **That an `SR-` test has a negative control.** The single most important rule
  in this repository, and there is no mechanical form of "someone watched this
  go red". It is a review question and a PR-template checkbox, and it stays
  there rather than being replaced by a proxy metric that could be satisfied
  without doing it.
- **Coverage thresholds.** A number that is gamed by testing what is easy.

Naming these is part of the decision. A list of automated checks reads like a
guarantee of quality; the three above are where the quality actually lives, and
they are guarded by people.

## Consequences
- CI is slower and has more jobs than a library this size normally carries. That
  is the intended trade: the alternative is discovering the same four failures
  this ecosystem has already paid for.
- A single `CI OK` gate job is what branch protection requires, because
  protection can only name jobs and a matrix job's name changes when the matrix
  does — so protecting matrix entries directly means adding a module silently
  widens the gap between "CI passed" and "everything ran".
- Every check has an escape hatch that is **visible**: `cross-module-window`
  for a PR that must change a core API and its consumer together, `adr-typo`
  for a genuine typo in an accepted ADR. A bypass a human has to spell out on
  the PR is different in kind from a default that lets things through.
- New conventions arrive with their check, or they arrive as documentation and
  are labelled as such.
