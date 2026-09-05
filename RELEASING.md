# Releasing

Modules are versioned and tagged independently (ADR-0001, NFR-03). The tag
prefix selects the module; [`.github/workflows/release.yml`](.github/workflows/release.yml)
does the rest.

| Module | Tag | Example |
| --- | --- | --- |
| `.` (core) | `vX.Y.Z` | `v0.1.0` |
| `redisstore/` | `redisstore/vX.Y.Z` | `redisstore/v0.1.0` |

`*` does not match `/` in a GitHub tag filter, so `v*` is the core alone.

**The order is not optional.** Read the next section before tagging anything.

---

## Why order matters

`redisstore` requires the core. Until the core version it names is published,
that requirement resolves to nothing for anyone outside this repository:

- A local `go.work` makes the satellite resolve the core from the working tree,
  so every local signal is green regardless of what the `require` line says.
- **A consumer ignores a dependency's `replace` directives**, and there is no
  workspace on their machine. They get exactly the version `redisstore/go.mod`
  names, from the proxy.

Tagging `redisstore` before the core version it requires exists therefore
publishes a release nobody can `go get`. This is not hypothetical: `moat`
shipped `redisstore/v0.2.0` this way, requiring a core whose `ratelimit.Store`
declared `Take` where the satellite implemented `TakeN`, and `crier` recorded
the same class of failure as audit finding A-9.

CI has a job for it — `satellite-resolution` in
[`ci.yml`](.github/workflows/ci.yml) runs with `GOWORK=off`, which is the only
resolution that can catch a stale pin. **It cannot help you if you tag in the
wrong order**, because the pin is only stale relative to a version that does
not exist yet.

### The sequence

```
1. tag + push the core      →  release.yml publishes it
2. wait for the proxy       →  go list -m github.com/JonasBorgesLM/cairn@vX.Y.Z
3. bump redisstore/go.mod   →  require the core at the version just published
4. let CI go green          →  satellite-resolution now proves the real pin
5. tag + push redisstore
```

Done in that order there is no window, because the core version the satellite
names is already published when the bump is committed.

---

## Before the first tag of a module

Some decisions become expensive the moment a version exists.

**The public API freezes.** Everything exported at the first tag is something
consumers may depend on. Walk `go doc -all ./...` and ask, of each exported
name, whether it is the shape you want to keep. Unexported is cheap now and
expensive later.

**`.github/allowed_signers` must be populated**, or nothing releases — the
`release` job fails closed on an empty file. See the instructions inside it.
Confirm signing works before you need it:

```bash
git tag -s v0.0.1-rehearsal -m "rehearsal"
git config gpg.ssh.allowedSignersFile .github/allowed_signers
git verify-tag v0.0.1-rehearsal
git tag -d v0.0.1-rehearsal
```

**Private vulnerability reporting must be on.** The window where someone finds
a hole and has nowhere private to report it starts at the first release, not at
the first user:

```bash
gh api repos/JonasBorgesLM/cairn/private-vulnerability-reporting
# {"enabled":true}
```

It was found disabled during both the `moat` and the `crier` releases, which is
twice, so it is a checklist item rather than an assumption.

**The `v1` key schema freezes with `redisstore`.** `cairn:v1:link:{code}` and
its siblings (SR-22) become a format that deployed data is written in. Changing
the record layout after that is a migration, not an edit.

---

## Which branch

**Tags are cut on `main`, and every commit in a release is made there.**

`develop` is where work lands; `main` is what has been released, and it is what
GitHub shows a visitor. Those being the same thing is deliberate — see
[`CONTRIBUTING.md`](CONTRIBUTING.md#develop-and-main).

Step 3 above — the `redisstore/go.mod` bump — is a release commit, so it is
made on `main`, and **`main` is merged back into `develop` when the release
finishes.** That last step is not bookkeeping: without it the two branches
diverge in the exact file the release exists to change. `crier`'s `main` held
nothing but an initial commit through six milestones precisely because nobody
had written down when it should have been updated.

---

## The procedure

```bash
# 0. On main, up to date, CI green.
git checkout main && git pull
gh run list --branch main --limit 1        # must be green

# 1. Documentation checks pass locally too -- the release job runs them, and
#    finding out there costs a tag you cannot take back.
./.github/scripts/check-docs.sh

# 2. Confirm what is about to be released.
go doc -all ./... | head -50               # the API being frozen
git log --oneline "$(git describe --tags --abbrev=0 2>/dev/null || echo HEAD~20)"..HEAD

# 3. Tag, signed.
git tag -s v0.1.0 -m "cairn v0.1.0"
git push origin v0.1.0

# 4. Watch it.
gh run watch
```

The workflow then, **before the release exists**:

- resolves the module from the tag, and fails if any module in the tree matches
  no tag pattern;
- builds, vets and tests with `GOWORK=off` — the resolution a consumer gets;
- runs `go mod verify` and `govulncheck`;
- re-checks the core's dependency policy, because a published version carrying
  a third-party dependency cannot be taken back;
- runs the documentation checks;
- generates the API diff with `gorelease` against this module's previous tag
  (NFR-13: the changelog is generated from what actually changed, not from what
  the author meant to change);
- verifies the tag signature against `.github/allowed_signers`;
- and only then creates the release.

A tag is immutable once anyone has fetched it, which is why everything that can
fail is checked before the release exists rather than after.

---

## Rehearsing it without publishing

Worth doing once before the first real tag. Push a tag to a scratch fork, or
run the verification steps by hand:

```bash
GOWORK=off go mod verify && GOWORK=off go build ./... && GOWORK=off go test -race ./...
go install golang.org/x/exp/cmd/gorelease@latest && gorelease -base=none
```

Then, from a directory **outside this repository**, prove a consumer can
actually use it:

```bash
cd "$(mktemp -d)" && go mod init scratch
go get github.com/JonasBorgesLM/cairn@v0.1.0
go get github.com/JonasBorgesLM/cairn/redisstore@redisstore/v0.1.0
go build ./...
```

That last block is the check `usher`'s ADR-11 established after a published
satellite tag turned out not to compile. Every local signal was green when it
was cut.

---

## If a release is wrong

**Do not delete or move the tag.** Anyone who fetched it has it, and the module
proxy has cached it permanently — a moved tag gives two people different code
under one version, which is worse than a bad version.

Cut the next patch. If the bad version must not be used, `retract` it in
`go.mod` and release again:

```go
retract v0.1.0 // save could overwrite an existing code
```
