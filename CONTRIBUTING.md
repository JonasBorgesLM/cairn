# Contributing to cairn

## Status

`cairn` is pre-implementation. Work is tracked on the
[project board](https://github.com/users/JonasBorgesLM/projects/4), grouped into
milestones M0 through M9.

---

## Git flow

### `develop` and `main`

**`develop` is where work lands. `main` is what has been released.**

- Every pull request targets `develop`. Nothing targets `main` directly.
- At each release, `develop` merges into `main` and the tags are cut there
  (see [`RELEASING.md`](RELEASING.md)). `main` is therefore always a released
  state, and it is also what GitHub shows a visitor — those being the same
  thing is the point.
- **Until the first tag exists**, `main` tracks `develop`: there is no released
  state for it to hold yet, and a default branch showing an empty project is
  worse than one showing unreleased work.
- Release commits — the `redisstore/go.mod` bump that swaps a working-tree
  resolution for a published version — are made on `main`, and **`main` is
  merged back into `develop` when the release finishes.**

That last step is not bookkeeping. Without it the branches diverge in the exact
file the release exists to change. `crier`'s `main` held only an initial commit
through six milestones because nobody had written down when it should have been
updated.

### Branches

```
feat/<short-name>        feat/random-code-generator
fix/<short-name>         fix/redisstore-setnx
docs/adr-<nnnn>          docs/adr-0017
ci/<short-name>          ci/pin-gosec
```

Branch from `develop`. One branch, one subject.

### Stacked pull requests

Milestones stack: while `feat/m1-core-types` is open, the M2 branch is based on
it rather than on `develop`, so its PR shows the M2 diff alone. That is worth
keeping — but the merge has an order, and getting it wrong costs a recovery.

**Retarget the dependent PR to `develop` before deleting the base branch.**

```bash
gh pr merge 42 --merge                 # merge the lower PR, keep the branch
gh pr edit 43 --base develop           # retarget the dependent PR FIRST
git push origin --delete feat/m1-core-types
```

Deleting the base branch while another PR is based on it **closes** that PR, and
the state is then stuck in both directions: you cannot retarget a closed PR, and
you cannot reopen one whose base branch is gone. Recovery is to restore the
branch at the commit the PR was based on, then unwind in the order the API
allows — reopen, retarget, then delete.

### Merging

- Squash for a branch whose intermediate commits are noise; merge commit for one
  whose history is worth keeping. Either way the resulting subject is a
  Conventional Commit, which CI checks on the PR title as well as on the commits.
- Delete the branch after merge, unless another PR is based on it.

---

## Commits

**[Conventional Commits](https://www.conventionalcommits.org)** (NFR-13),
enforced by the `commits` job in CI on every commit in a pull request and on the
PR title.

```
<type>(<scope>)!: <subject>

<body: why, not what>

Refs SR-18
Closes #33
```

| | |
| --- | --- |
| **Types** | `feat` `fix` `docs` `test` `refactor` `perf` `build` `ci` `chore` `revert` |
| **Scopes** | `cairn` `policy` `memstore` `cairnhttp` `redisstore` `adr` `docs` `deps` `ci` `security` |
| **Subject** | imperative, ≤ 72 characters, no trailing period |
| **Breaking** | `!` before the colon **and** a `BREAKING CHANGE:` footer |

```
feat(policy): reject IPv4-mapped IPv6 destinations
fix(redisstore): use SetNX rather than SET on save
docs(adr): record the redirect status decision
ci: pin gosec to an exact version
```

**Explain why in the body.** The diff already says what. Cite the requirement or
ADR — those ids are how someone a year from now finds the reasoning, and CI
checks that every id cited anywhere resolves to one that exists.

**One subject per commit, staged explicitly.** Do not use `git add -A` when the
working tree holds work on more than one subject. A commit whose message does
not describe everything in it cannot be reviewed or reverted cleanly. When
several subjects have already been committed together, `git reset --soft HEAD~1`
and explicit staging is the fix — not an amended message that lists them.

---

## Documentation is part of the change

Not a follow-up, and not a nice-to-have. Requirements are cited by id from
ADRs, godoc, tests and commit messages, so documentation here is load-bearing
structure rather than prose around the code.

Four things are enforced mechanically, and each answers a way this has actually
rotted:

1. **Exported identifiers carry doc comments** (`revive`'s `exported` rule in
   [`.golangci.yml`](.golangci.yml)). Several of cairn's contracts live nowhere
   but the godoc: that `Save` must not overwrite, that `Revoke` does not
   authorize, that a `CodeGenerator` must draw from a CSPRNG.
2. **Every cited requirement and threat id exists.** A citation of a renumbered
   requirement is a reader following a reference to nothing. Cross-project
   citations are written qualified — `crier/IR-06`, `moat/NFR-01` — because
   unqualified they would be silently checked against cairn's numbering.
3. **Every relative link between documents resolves.** The cheapest kind of rot.
4. **Every public package exporting an API has an `ExampleXxx`** (NFR-09).

Run them locally before pushing — the same script CI runs:

```bash
./.github/scripts/check-docs.sh
```

### ADRs are never rewritten

NFR-14. An ADR records the reasoning that was current when a decision was made.
Editing it destroys exactly what it exists to preserve.

A change of mind is **a new ADR that supersedes the old one**, or an
`## Amendment` section appended to it naming what superseded which part. CI
enforces the mechanical version of this: an existing ADR may gain lines and
never lose them. A genuine typo fix is the one exception — label the PR
`adr-typo`.

Write the ADR **before** the code. A question listed as open in
[`docs/adr/README.md`](docs/adr/README.md) must not be resolved silently by an
implementation; an undecided question that looks decided is the one that gets
implemented by accident.

---

## Before you write code

1. Find the requirement id (`FR-`, `SR-`, `NFR-`, `IR-`) your change implements.
   If there is not one, the change needs a requirement first.
2. If the change is structural, write the ADR first.
3. If it touches an `SR-`, plan the negative control before the test.

---

## Per-module commands

Multi-module repository (ADR-0001). A green build in one module says nothing
about the other.

```bash
go work init . ./redisstore          # once; go.work is not committed

for m in . redisstore; do
  (cd "$m" && go build ./... && go vet ./... && go test -race ./...)
done

golangci-lint run ./...
gosec -tests -exclude-generated ./...
govulncheck ./...
./.github/scripts/check-docs.sh
```

`go.work` is deliberately not committed: a committed workspace changes how
modules resolve for everyone who clones the repository, and it would make the
`satellite-resolution` CI job — the one that catches a stale pin — prove nothing.

---

## Testing rules

- Table-driven, `t.Run` subtests, asserting the specific behaviour.
- Every public package has `ExampleXxx` functions (NFR-09).
- Integration tests use testcontainers against a real Redis (NFR-10). Never
  miniredis: a fake that implements `SetNX` correctly proves nothing about the
  server that has to.
- **Every `SR-` test carries a negative control.** Remove the protection, watch
  the test fail, restore it, and note the control in a comment above the test:

  ```go
  // SR-18: Save must not overwrite an existing code.
  // Negative control: verified failing against a store whose Save uses SET.
  ```

  A test that passes against a broken implementation is worse than no test,
  because someone will cite it. If you cannot make a check fail on demand, say so
  in the PR rather than claiming coverage.
- **A negative assertion is satisfied by a tool that never ran.** Assert the
  expected answer, not the absence of a wrong one.

---

## Reviewing

In this order:

1. Which requirement does this implement, and does the code match its wording?
2. Which threat does it touch, and is the residual still what
   [`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md) says it is?
3. Has the `SR-` test been **seen to fail**?
4. Does it violate an invariant in [`CLAUDE.md`](CLAUDE.md)?
5. Does it add a dependency to the core module? (The answer is no.)

### Reviewing a Dependabot pull request

A dependency bump is a supply-chain change with the same blast radius as a
hand-written commit. Read the changelog between the two versions, not just the
version numbers. If the diff is unreadable, that is a finding, not a reason to
merge.

Dependabot is configured **not** to touch `moat` (ADR-0007). A PR bumping it
means the ignore rule was removed — that is the review.

---

## Reporting a vulnerability

See [`SECURITY.md`](SECURITY.md). Do not open a public issue.
