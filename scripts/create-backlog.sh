#!/usr/bin/env bash
#
# cairn — creates labels, milestones, issues and the GitHub Project board.
#
# Idempotent for labels and milestones. NOT idempotent for issues: running it
# twice creates duplicates. Use --dry-run first.
#
# Usage:
#   chmod +x scripts/create-backlog.sh
#   ./scripts/create-backlog.sh --dry-run
#   ./scripts/create-backlog.sh
#
# Requires: gh CLI, authenticated, with the 'project' scope.
#   gh auth refresh -s project

set -euo pipefail

DRY_RUN=0
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

command -v gh >/dev/null 2>&1 || { echo "gh CLI not found."; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh CLI not authenticated. Run: gh auth login"; exit 1; }

REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
OWNER=${REPO%%/*}
PROJECT_TITLE="cairn"

echo "Repository: $REPO"
[[ $DRY_RUN -eq 1 ]] && echo "DRY-RUN — nothing will be created"
echo

# ---------------------------------------------------------------- labels ----

label() {  # name color description
  [[ $DRY_RUN -eq 1 ]] && { printf '  [dry-run] label %s\n' "$1"; return; }
  gh label create "$1" --color "$2" --description "$3" --force --repo "$REPO" >/dev/null
  printf '  ok %s\n' "$1"
}

echo "Labels..."
label security   "b60205" "Implements or protects a stated SR- requirement"
label core       "0e8a16" "Core module, cairn package"
label policy     "1d76db" "Destination policy"
label store      "5319e7" "Store interfaces and implementations"
label http       "fbca04" "cairnhttp"
label docs       "0075ca" "Documentation and ADRs"
label ci         "444444" "Build, lint, release automation"
label testing    "c2e0c6" "Tests, benchmarks, verification"
label release    "d4c5f9" "Versioning and release"
label adr-needed "e99695" "Blocked on an ADR being written first"
echo

# ------------------------------------------------------------ milestones ----

milestone() {  # title description
  if [[ $DRY_RUN -eq 1 ]]; then printf '  [dry-run] milestone %s\n' "$1"; return; fi
  local existing
  existing=$(gh api "repos/$REPO/milestones?state=all&per_page=100" --jq ".[] | select(.title==\"$1\") | .number" 2>/dev/null || true)
  if [[ -n "$existing" ]]; then
    printf '  exists %s (#%s)\n' "$1" "$existing"
    return
  fi
  gh api "repos/$REPO/milestones" -f title="$1" -f description="$2" >/dev/null
  printf '  ok %s\n' "$1"
}

echo "Milestones..."
milestone "M0 Foundation"        "Module skeletons, CI, lint, dependency guard. No domain code."
milestone "M1 Core types"        "Code, Alphabet, Destination, Link, errors, Hooks."
milestone "M2 Code generation"   "CodeGenerator, CSPRNG generator, keyspace density validation."
milestone "M3 Destination policy" "Policy interface and the policy/ implementations."
milestone "M4 Shortener"         "Create, Resolve, Revoke, retry, memstore."
milestone "M5 redisstore"        "go-redis store, Lua atomicity, eviction check, testcontainers."
milestone "M6 cairnhttp"         "Redirect handler, ErrorEncoder, interstitial."
milestone "M7 Extras"            "Vanity, ownership listing, counter, deduplication."
milestone "M8 Verification"      "Threat probes, negative-control audit, benchmarks, demo."
milestone "M9 Release"           "task-api integration, changelog, v0.1.0."
echo

# ---------------------------------------------------------------- issues ----

ISSUE_URLS=()

# Body comes in on stdin (heredoc), not as an argument: a heredoc inside $( )
# breaks in macOS bash 3.2 when the text contains an apostrophe.
issue() {  # title milestone labels  <<BODY
  local title="$1" ms="$2" labels="$3" body
  body=$(cat)
  if [[ $DRY_RUN -eq 1 ]]; then
    printf '  [dry-run] %-62s [%s] {%s}\n' "$title" "$ms" "$labels"
    return
  fi
  local url
  url=$(gh issue create --repo "$REPO" --title "$title" --body "$body" \
        --milestone "$ms" --label "$labels")
  ISSUE_URLS+=("$url")
  printf '  %s  %s\n' "${url##*/}" "$title"
}

echo "Issues..."

# ---- M0 --------------------------------------------------------------------

issue "Bootstrap the core module" "M0 Foundation" "ci,core" <<'BODY'
Create the core `go.mod` as `github.com/JonasBorgesLM/cairn`, `go 1.24` (NFR-02),
with `github.com/JonasBorgesLM/moat` as its only `require`, pinned to an exact
version (ADR-0001, ADR-0007).

Also: package directories with `doc.go` only — `policy/`, `memstore/`,
`cairnhttp/`. No types yet.

**Done when**
- [ ] `go build ./...` passes with no Go files beyond `doc.go`
- [ ] `go.mod` requires exactly one module, and it is first-party
- [ ] `moat` is pinned to an exact version, not a range
BODY

issue "Bootstrap the redisstore module" "M0 Foundation" "ci,store" <<'BODY'
`redisstore/go.mod` as `github.com/JonasBorgesLM/cairn/redisstore` (ADR-0001,
NFR-03), with go-redis and testcontainers (test only).

The `go` directive records whatever the dependencies impose, with a comment
saying it is an **imposition and not a choice** — the wording `crier`'s NFR-02
uses, so nobody later reads the higher floor as an endorsement.

**Done when**
- [ ] `(cd redisstore && go build ./...)` passes
- [ ] A non-committed `go.work` recipe is documented, following `moat`
BODY

issue "CI: per-module build, vet, race tests" "M0 Foundation" "ci" <<'BODY'
`.github/workflows/ci.yml` running, **for each module independently** (NFR-15):
`go build ./...`, `go vet ./...`, `go test -race ./...`.

A single job that runs at the repository root does not descend into the other
module even inside a workspace. A green build in one module says nothing about
the other — a matrix, not a loop in one job.

**Done when**
- [ ] Core and `redisstore` are separate matrix entries
- [ ] A deliberate failure in `redisstore` has been seen to fail CI
BODY

issue "CI guard: the core module takes no third-party dependency" "M0 Foundation" "ci,security" <<'BODY'
NFR-01 and ADR-0001: the core `go.mod` may require nothing outside
`github.com/JonasBorgesLM/`. A policy that is not checked is a preference.

Fail the build on any other `require`.

**Done when**
- [ ] The check fails when a third-party require is added — **verified by adding
      one and watching it go red**, then removing it
- [ ] The failure message names ADR-0001
BODY

issue "golangci-lint configuration" "M0 Foundation" "ci" <<'BODY'
`.golangci.yml` (golangci-lint v2), following `crier`'s configuration as the
starting point. Runs on the core in CI (NFR-15).

**Done when**
- [ ] `golangci-lint run ./...` passes in the core
- [ ] The config is committed, not implied by defaults
BODY

issue "CI: govulncheck" "M0 Foundation" "ci,security" <<'BODY'
`govulncheck ./...` per module (NFR-15), run on the supported toolchain rather
than at the library floor — a library verified only at its floor is scanned
against a standard library that no longer receives fixes (`crier` NFR-02).

**Done when**
- [ ] Runs for both modules
- [ ] A finding fails the build rather than warning
BODY

issue "Dependabot: exclude moat from automated bumps in the core" "M0 Foundation" "ci,security" <<'BODY'
ADR-0007: `moat` is pinned exactly and upgraded manually, verified with a build
from a clean directory outside the repository — the procedure `usher`'s ADR-11
established after a breaking rename in a minor release and a satellite tag that
did not compile.

An automated bump of this dependency is precisely the event ADR-0007 exists to
prevent.

**Done when**
- [ ] `.github/dependabot.yml` covers both modules
- [ ] `moat` is explicitly ignored in the core module's entry, with a comment
      pointing at ADR-0007
BODY

issue "Create the develop branch and protect main" "M0 Foundation" "ci" <<'BODY'
NFR-16: feature branches merge to `develop` via PR; `main` is the release branch.

**Done when**
- [ ] `develop` exists and is the default branch for PRs
- [ ] `main` requires a PR and passing CI
BODY

# ---- M1 --------------------------------------------------------------------

issue "Code and Alphabet types with validation" "M1 Core types" "core,security" <<'BODY'
FR-08, SR-21. `Code` as a string type; `Alphabet` as a validated set.

`NewAlphabet` rejects duplicates, non-ASCII runes, and anything outside
`[A-Za-z0-9_-]` — a rune needing percent-encoding in a path segment makes the
code's own rendering ambiguous, which defeats SR-21's premise that a code is
validatable before it becomes a key (ADR-0004).

`Alphabet.Validate(Code)` is the function SR-21 depends on.

**Done when**
- [ ] `Validate` rejects runes outside the alphabet, over-long and empty codes
- [ ] `ExampleAlphabet_Validate` exists (NFR-09)
- [ ] Table-driven tests including `:`, `*`, newline, and a NUL byte
BODY

issue "AlphabetBase62 and AlphabetUnambiguous, with the entropy floor preserved" "M1 Core types" "core,security" <<'BODY'
ADR-0004. base62 (5.954 bits/rune) is the default at length 10;
`AlphabetUnambiguous` drops `0 O o 1 l I` (56 runes, 5.807 bits/rune) and
**raises the default length to 11**, so choosing legibility never quietly trades
away the security parameter of ADR-0002.

**Done when**
- [ ] Selecting `AlphabetUnambiguous` without an explicit length yields 11
- [ ] An explicit `WithCodeLength` still wins, and is still density-checked
- [ ] `BitsPerRune` is tested against the documented values
- [ ] The length interaction is documented at **both** option sites
BODY

issue "Destination built on moat/secret, with no unredacted format path" "M1 Core types" "core,security" <<'BODY'
SR-15, ADR-0007, IR-03. `Destination` holds the raw URL in a `secret.Value`,
plus `scheme` and `host` in the clear (what an operator needs to triage; neither
carries the secret).

Redacted rendering is `https://example.com/[REDACTED]` through **every** path:
`String`, `GoString`, `Format`, `LogValue`, `MarshalText`, `MarshalJSON`.
`URL()` and `Raw()` are the only ways out.

`UnmarshalText`/`UnmarshalJSON` refuse a value containing the placeholder,
mirroring `secret.ErrRedacted`, so a JSON round trip cannot launder a redacted
rendering back into a live destination.

**Done when**
- [ ] A test enumerates every exported method and asserts none emits the query
      string — including `%v`, `%s`, `%q`, `%+v`, `%#v`, `json.Marshal`,
      `slog` with a `Destination` inside a struct
- [ ] Negative control: the test has been seen to fail with `String()` removed
- [ ] `ExampleDestination` shows the redaction
BODY

issue "ParseDestination: syntax and shape checks" "M1 Core types" "core,security" <<'BODY'
SR-05, SR-06, SR-08, SR-10. No network, no context — those belong to `Policy`
(ADR-0005).

- Length checked **before** parsing (SR-08, default 2000 bytes)
- Scheme allowlist `http`/`https` — an allowlist, never a denylist (SR-05)
- Userinfo rejected (SR-06): `https://accounts.example.com@evil.example/`
- Control characters and whitespace rejected anywhere, including percent-decoded
  forms in the host (SR-10)
- Non-ASCII hostnames rejected in v1 (ADR-0015 — punycode needs `x/net/idna`,
  which the core may not import)

**Done when**
- [ ] Each rejection has its own `RejectReason`
- [ ] Test vectors include `javascript:`, `data:`, `file:`, `\r\n` injection,
      `https://a@b@c/`, and a percent-encoded newline in the host
- [ ] Negative controls recorded for SR-05 and SR-06
BODY

issue "URL normalization, minimal and semantics-preserving" "M1 Core types" "core" <<'BODY'
FR-16, ADR-0015. Apply only RFC 3986 §6.2.2 equivalences: lowercase scheme and
host, remove default port, decode unreserved percent-encodings and uppercase hex
digits, empty path becomes `/`, resolve dot segments.

**Do not** sort query parameters, strip trailing slashes, lowercase the path,
remove `www.`, drop the fragment, or remove tracking parameters. Each of those
breaks a real server, and ADR-0015 says which.

Runs after parse-time validation and before policy evaluation, so policy always
sees the canonical form.

**Done when**
- [ ] A table asserts each applied rule and each **rejected** rule
- [ ] A pre-signed-URL-shaped case proves query order survives
BODY

issue "Link type and lifecycle predicates" "M1 Core types" "core" <<'BODY'
ADR-0009, ADR-0010. `Link{Code, Dest, OwnerID, Vanity, CreatedAt, ExpiresAt,
RevokedAt}` with `IsExpired`, `IsRevoked`, `IsActive`.

`ExpiresAt` is a logical expiry evaluated against the configured clock, not the
store's — a store's clock is not cairn's. Native TTL is the garbage collector;
the field is the authority.

`OwnerID` is data. cairn compares and stores it and attaches no meaning to it.

**Done when**
- [ ] Predicates are tested at the exact boundary instant
- [ ] Zero `ExpiresAt` means never, tested explicitly
- [ ] Godoc on `OwnerID` states that cairn never uses it for authorization
BODY

issue "Sentinel errors and RejectReason" "M1 Core types" "core" <<'BODY'
NFR-07. The set in `docs/ARCHITECTURE.md` §2.4, all comparable with `errors.Is`.

`ErrDestinationRejected` wraps a `RejectReason` so a host can render a specific
message without matching on strings, while `errors.Is(err, ErrDestinationRejected)`
stays true.

**Done when**
- [ ] Every sentinel has an `errors.Is` test
- [ ] Wrapping preserves both the sentinel and the reason
- [ ] `RejectReason` values are documented as a stable enum safe to expose in an
      API response (IR-04)
BODY

issue "Hooks struct and event types" "M1 Core types" "core" <<'BODY'
FR-17, ADR-0016. A struct of nil-able callbacks, not an interface: an interface
forces a consumer who wants one event to implement four, and a fifth event later
would break every implementer.

Events carry `Destination` (which redacts itself — SR-15) and typed
`RejectReason`, **never raw URL strings**. That is what makes a whole event safe
to hand to an arbitrary logger.

**Done when**
- [ ] A nil field is a no-op, tested
- [ ] No event struct has a `string` field carrying a destination
- [ ] Godoc states, in the imperative, that handlers are synchronous and must
      not block
BODY

# ---- M2 --------------------------------------------------------------------

issue "CodeGenerator interface" "M2 Code generation" "core,security" <<'BODY'
FR-07. One method. The godoc is the deliverable as much as the signature: an
implementation MUST draw from a CSPRNG, and a counter, a timestamp, a hash of
the destination, or `math/rand` each reintroduce T-02 — the last while looking
random.

This is the most dangerous interface in the library to implement casually, and
the documentation says so in those terms.

**Done when**
- [ ] Godoc carries the warning and links ADR-0002
BODY

issue "CSPRNG code generator with rejection sampling" "M2 Code generation" "core,security" <<'BODY'
SR-01, ADR-0002. `crypto/rand` with **rejection sampling**, not modulo
reduction: `b % 62` over a uniform byte biases the first eight runes upward,
shrinking the effective keyspace for free and for no reason.

**Done when**
- [ ] A chi-square test over a large sample shows no rune bias
- [ ] Negative control: the same test has been seen to **fail** against a
      deliberately modulo-biased generator, and that fact is recorded in a
      comment above it
- [ ] A `crypto/rand` read error propagates and never yields a partial code
- [ ] `ExampleNewRandomGenerator`
BODY

issue "Keyspace density validation at New" "M2 Code generation" "core,security" <<'BODY'
SR-02, T-10, ADR-0002. `WithExpectedLinks(n)` feeds the check; `New` **refuses**
a configuration whose `E[hits] = R·N/A^L` is self-evidently unsafe at the
declared `n`, with the arithmetic in the error message.

A runtime warning is not enough: a consumer who sets `WithCodeLength(5)` for
prettier links has to be stopped, not informed.

**Done when**
- [ ] `WithCodeLength(5)` with a million expected links fails at `New`
- [ ] The error message contains the computed number, not just a verdict
- [ ] The formula is in the package documentation so a host with 10^10 links can
      compute their own length
BODY

# ---- M3 --------------------------------------------------------------------

issue "Policy interface, Decision, and Chain" "M3 Destination policy" "policy,security" <<'BODY'
ADR-0005. `Policy` and `Decision` are declared in the **core**; implementations
live in `policy/`, which imports `cairn`. The reverse is an import cycle, and
the Go idiom — the consumer declares the interface — resolves it.

The visible consequence is deliberate: `cairn.New` cannot default to
`policy.Default()`, so it does not default at all. **No policy is a construction
error.** A shortener without destination policy is what this library exists not
to be, and the layout makes that unrepresentable rather than discouraged.

`Chain` composes first-non-`Allow`-wins, with `Deny` beating `Interstitial`, so
no ordering can downgrade a rejection into a warning.

**Done when**
- [ ] `New` without a policy returns an error naming the requirement
- [ ] `Chain(Allowlist, BlockPrivate)` and the reverse order both `Deny` a
      private address — ordering cannot produce `Interstitial`
BODY

issue "policy: SchemeAllowlist, MaxLength, BlockOwnDomains" "M3 Destination policy" "policy,security" <<'BODY'
SR-05, SR-08, SR-09.

`BlockOwnDomains` covers the anti-loop requirement and covers **only the host's
own domains**. Chained third-party shorteners are explicitly not addressed —
maintaining a denylist of shortener domains is a losing game, and the godoc says
so rather than implying coverage.

**Done when**
- [ ] Subdomain matching is tested both ways: `a.example.com` matches
      `example.com`, `notexample.com` does not
- [ ] Case and trailing-dot forms of the host are handled
BODY

issue "policy.BlockPrivateNetworks with an injectable Resolver" "M3 Destination policy" "policy,security" <<'BODY'
SR-07, T-05. Block loopback, RFC 1918, link-local (incl. `169.254.169.254` and
`fe80::/10`), CGNAT `100.64.0.0/10`, unspecified, multicast, IPv6 ULA `fc00::/7`.

Both literal addresses in the host **and** the hostname's resolved addresses.

`Resolver` is an interface so this is testable without a network. A test that
needs DNS is a test that will be skipped.

Fail-closed: a resolver error returns `Deny` with `ReasonResolveFailed`. Failing
open would make DNS unavailability a temporary permission to shorten internal
addresses (SR-20).

**Done when**
- [ ] Each blocked range has its own test case
- [ ] A resolver returning an error produces `Deny`, not `Allow` — negative
      control recorded
- [ ] The godoc carries the TOCTOU paragraph from ADR-0005, where an IDE
      autocomplete will show it
BODY

issue "policy: reject IPv4-mapped IPv6 and non-decimal IPv4 literals" "M3 Destination policy" "policy,security" <<'BODY'
SR-07, and the bypass half of it.

`http://[::ffff:127.0.0.1]/` is loopback in a different notation; a checker that
only knows IPv4 literals waves it through.

`0x7f.0.0.1`, `2130706433`, `0177.0.0.1` are **rejected outright, not parsed and
classified**. Every parser disagrees about them, and the disagreement between
cairn's parser and the victim's is exactly the bypass. Refusing a form nobody
legitimately writes costs nothing.

**Done when**
- [ ] Hex, octal, decimal-integer and mixed-shorthand IPv4 forms are all rejected
- [ ] `::ffff:` mapped forms are classified as their IPv4 equivalent
- [ ] Negative control recorded for each
BODY

issue "policy.Allowlist returning Interstitial" "M3 Destination policy" "policy" <<'BODY'
FR-13. Domains on the list `Allow`; everything else `Interstitial`, not `Deny` —
that is what makes ADR-0014 possible without a second classification pass.

**Done when**
- [ ] An empty allowlist means everything is `Interstitial`, and that is
      documented as intentional rather than a footgun
BODY

issue "policy.Default composition" "M3 Destination policy" "policy,docs" <<'BODY'
ADR-0005. `Default(ownDomains)` chains the checks in the order the create flow
documents. The one call a consumer makes if they have not thought about it, so
it must be the safe one.

**Done when**
- [ ] `ExamplePolicy_default` shows the full safe setup in under ten lines
- [ ] A test asserts the composed set matches SR-05 through SR-10 exactly, so a
      requirement cannot be dropped from the default without a test failing
BODY

# ---- M4 --------------------------------------------------------------------

issue "Store interface and optional capability interfaces" "M4 Shortener" "store,security" <<'BODY'
FR-04, ADR-0008. Three required methods — `Save`, `Load`, `Revoke` — because
every method here is one every implementer must get right.

`OwnerLister`, `DestIndex`, `EvictionChecker`, `Counter` are optional
capabilities, type-asserted **at construction**, never per call, so a missing
capability is a startup error rather than a runtime surprise.

`Save`'s godoc states in those words that an implementation which overwrites is
a **defect, not a variation** (SR-18).

**Done when**
- [ ] A `Store` that is not a `DestIndex` fails `New` when dedup is enabled
- [ ] The interface documentation carries the conditional-write contract
BODY

issue "Shortener.New with eager validation" "M4 Shortener" "core" <<'BODY'
NFR-08. Every misconfiguration fails at `New`, not at first use:

- no `Policy` (ADR-0005)
- keyspace density unsafe for the declared expected links (ADR-0002)
- dedup enabled against a store that is not a `DestIndex` (ADR-0013)
- vanity length range overlapping the generated length (SR-04)
- an `EvictionChecker` store reporting an evicting policy (ADR-0008)

**Done when**
- [ ] One test per rejection, each asserting the message names the requirement
- [ ] No option can be set after construction — no exported setters (NFR-04)
BODY

issue "Create flow with bounded retry" "M4 Shortener" "core,security" <<'BODY'
FR-01, SR-18, SR-19. The flow in `docs/ARCHITECTURE.md` §5.1.

Two orderings are load-bearing and need their own tests:

1. **Save before dedup indexing.** A dangling index entry is recoverable; an
   index entry pointing at a link that was never saved resolves to nothing.
2. **Retry only for generated codes.** Retrying a vanity code means silently
   issuing a different code than the caller asked for.

**Done when**
- [ ] Retry is bounded by `WithMaxSaveAttempts` (default 5) and exhaustion
      returns `ErrCodeSpaceExhausted`
- [ ] A store that always returns `ErrCodeExists` terminates — negative control
      against an unbounded loop
- [ ] `Hooks.OnRetry` fires once per attempt (this is ADR-0003's measurement)
- [ ] A store error yields no link and no hook claiming success (SR-20)
BODY

issue "Resolve flow: validate before key construction" "M4 Shortener" "core,security" <<'BODY'
FR-02, SR-21. Alphabet and length validation come **before** the code reaches the
store — the ordering *is* the requirement, and reversing it produces code that
looks identical and is not.

It also makes a scanner's malformed probes free: they never reach the store.

Then: `ErrCodeNotFound` / `ErrLinkRevoked` / `ErrLinkExpired` as three distinct
outcomes (FR-09).

**Done when**
- [ ] An invalid code performs **zero** store round trips — asserted with a
      counting fake, not by inspection
- [ ] Negative control: the assertion has been seen to fail with the order
      reversed
- [ ] Each of the three outcomes has its own test
BODY

issue "Revoke, with an optional destination purge" "M4 Shortener" "core" <<'BODY'
FR-03, ADR-0009. `Revoke` sets `RevokedAt` and keeps the record, so a deliberate
act stays visible for the grace window — which is what a support conversation
and an audit trail both need.

`RevokeAndPurge` tombstones the code **and** clears the destination in the same
write, for the case where the reason to revoke is that the URL itself is toxic.
The plain variant keeps the destination, because the usual reason is that the
link is wrong, not dangerous.

`Revoke` does **not** check ownership (ADR-0010). The godoc says so in the
imperative, because that is the assumption a reader makes in the other direction.

**Done when**
- [ ] A revoked link resolves to `ErrLinkRevoked` on the very next call, with
      nothing to invalidate anywhere (SR-23)
- [ ] After purge, the record exists and the destination is empty
- [ ] The godoc warning is present and reviewed as the deliverable it is
BODY

issue "memstore: in-memory Store" "M4 Shortener" "store,testing" <<'BODY'
FR-05. For consumers testing their own integration without a container.

It implements the **same conditional-write contract** as `redisstore` (SR-18). A
memstore that overwrites would let a consumer's tests pass against behaviour the
real store forbids, which is worse than not shipping it.

**Done when**
- [ ] A shared conformance suite runs against both `memstore` and `redisstore`
- [ ] Concurrent `Save` of the same code: exactly one wins, tested under `-race`
- [ ] Implements `OwnerLister`, `DestIndex` and `Counter` so consumers can test
      every path
BODY

issue "Testable examples for the core package" "M4 Shortener" "docs,testing" <<'BODY'
NFR-09, matching `moat` and `crier`.

At minimum: `Example` (create → resolve → revoke against `memstore`),
`ExampleShortener_Create_vanity`, `ExampleDestination` (showing redaction),
`ExampleAlphabet_Validate`.

**Done when**
- [ ] `go test ./...` runs them and their `// Output:` matches
- [ ] The top-level `Example` is under 30 lines and needs no Redis
BODY

# ---- M5 --------------------------------------------------------------------

issue "redisstore: Store implementation with conditional Save" "M5 redisstore" "store,security" <<'BODY'
FR-06, SR-18. `SET key value NX`, never a plain `SET`.

**Done when**
- [ ] An integration test proves a second `Save` of the same code returns
      `ErrCodeExists` and leaves the original record byte-identical
- [ ] Negative control: verified failing against a build using `SET`
- [ ] Concurrent creation of the same code from many goroutines yields exactly
      one winner
BODY

issue "redisstore: namespaced, versioned key schema" "M5 redisstore" "store,security" <<'BODY'
SR-22, and `docs/ARCHITECTURE.md` §6.

```
cairn:v1:link:{code}      HASH   TTL = ExpiresAt + grace
cairn:v1:owner:{ownerID}  ZSET   code -> CreatedAt
cairn:v1:dest:{sha256}    STRING dedup index, only when enabled
cairn:v1:hits:{code}      STRING counter
```

The dedup index keys on `sha256(normalized destination)`, never the URL: key
names appear in `SCAN` output, `MONITOR` and slow-query logs, none of which are
covered by SR-15's redaction surface.

`{code}` reaches key construction already validated (SR-21).

**Done when**
- [ ] No key is built from an unvalidated code — asserted, not assumed
- [ ] A test greps the Redis keyspace after a create and asserts no key contains
      a URL fragment
BODY

issue "redisstore: Lua script for record and owner index atomicity" "M5 redisstore" "store" <<'BODY'
NFR-11, ADR-0010. The record and its owner-index entry are written in one script
so they cannot diverge under a partial failure. Same for removal.

The script lives in a `.lua` file loaded with `go:embed`, never a Go string
literal.

**Done when**
- [ ] `.lua` file, embedded, and readable on its own
- [ ] A killed connection mid-write leaves either both or neither — tested with
      testcontainers, not reasoned about
BODY

issue "redisstore: EvictionCheck at startup" "M5 redisstore" "store,security" <<'BODY'
ADR-0008, T-12. Implement `EvictionChecker`; `Shortener.New` calls it and
**refuses to start** against a `maxmemory-policy` other than `noeviction`.

Any `allkeys-*` policy silently deletes links under memory pressure: a memory
spike becomes permanent link loss with no error anywhere. Mirrors `moat`'s
rate-limit store check and `usher`'s ADR-3.

**Done when**
- [ ] A testcontainer configured `allkeys-lru` makes `New` fail
- [ ] The error names the requirement and the exact setting to change
- [ ] An unreachable Redis at startup fails closed, not open
BODY

issue "redisstore: error mapping without leaking go-redis types" "M5 redisstore" "store" <<'BODY'
NFR-07. `redis.Nil` becomes `ErrCodeNotFound`; connection and timeout failures
become `ErrStoreUnavailable`; the cause is wrapped so `errors.As` still reaches
it for a consumer who wants it.

No `go-redis` type appears in any exported signature.

**Done when**
- [ ] A test asserts every exported signature's types come from cairn or the
      standard library
- [ ] `errors.Is(err, cairn.ErrStoreUnavailable)` holds for a dead connection
BODY

issue "redisstore: testcontainers integration suite" "M5 redisstore" "testing,store" <<'BODY'
NFR-10. A real Redis, never miniredis — a fake that implements `SetNX` correctly
proves nothing about the server that has to.

Covers: conditional save under concurrency, TTL and the grace window (ADR-0009),
owner index atomicity, eviction check, error mapping, and the shared conformance
suite from the `memstore` issue.

**Done when**
- [ ] The suite runs in CI with the container, and is skipped with a clear
      message when Docker is absent — **skipped, not silently passing**
- [ ] Redis version is pinned
BODY

# ---- M6 --------------------------------------------------------------------

issue "cairnhttp: redirect handler with 302 and no-store" "M6 cairnhttp" "http,security" <<'BODY'
SR-11, SR-12, SR-14, ADR-0006.

- `302 Found` — never 301, at any configuration. There is no option to emit 301;
  an option to break revocation is a trap, not a feature.
- `Cache-Control: no-store` — 302 alone is heuristically cacheable under
  RFC 9111 §4.2.2, so the status choice is only half the mitigation.
- `Referrer-Policy: no-referrer` — otherwise the destination and every script on
  it learns the short code (T-13).
- `Pragma: no-cache` for HTTP/1.0-era intermediaries.

**Done when**
- [ ] Header assertions on the real response, not on a struct
- [ ] Negative control: the caching test has been seen to fail with `no-store`
      removed
- [ ] `grep -r '301'` finds no path that can emit one
BODY

issue "cairnhttp: 405 for non-GET/HEAD" "M6 cairnhttp" "http,security" <<'BODY'
SR-13. This is what removes the reason to consider 307/308: those statuses exist
to preserve a method across a redirect, and a shortener that never redirects a
non-GET has no method to preserve. 307 would also invite a client to replay a
request body to a destination the shortener does not control.

**Done when**
- [ ] POST, PUT, DELETE, PATCH all get 405 with a correct `Allow` header
- [ ] HEAD gets the redirect with no body
BODY

issue "cairnhttp: pluggable ErrorEncoder" "M6 cairnhttp" "http" <<'BODY'
FR-12, IR-04. The seam that lets `task-api` keep its own JSON error envelope.

The default maps per `docs/INTEGRATION.md` §4.2. The 410-versus-404 decision
belongs to the host, not to cairn (SR-03): distinguishing expired from not-found
helps a legitimate user and is an oracle to a scanner. That is precisely why this
is a function and not a table.

**Done when**
- [ ] `ErrStoreUnavailable` produces 503 and **never** a redirect, in the default
      and with a custom encoder that tries to
- [ ] An encoder that panics does not leak a partial redirect
- [ ] `ExampleWithErrorEncoder` shows the `task-api` envelope
BODY

issue "cairnhttp: header ordering under a moat middleware chain" "M6 cairnhttp" "http,security,testing" <<'BODY'
IR-01, and the overlap named in `docs/INTEGRATION.md` §2.2.

`moat`'s `secureheaders` sets headers across all responses; `cairnhttp` sets
`Cache-Control: no-store` and `Referrer-Policy: no-referrer` on the redirect.
Both write the same map, and last-writer-wins depends on middleware order.

`cairnhttp` must set its two headers **immediately before `WriteHeader`**, so a
surrounding middleware cannot overwrite the ones revocation and SR-14 depend on.

This test would otherwise never be written, because each component is correct
alone.

**Done when**
- [ ] A test wraps the handler in a real `moat` `secureheaders` chain and asserts
      both headers survive
- [ ] Negative control: seen to fail with the headers set at handler entry
BODY

issue "cairnhttp: interstitial page, continuation by code only" "M6 cairnhttp" "http,security" <<'BODY'
FR-13, SR-24, ADR-0014.

**The failure mode this issue exists to prevent:** `/warn?to=<destination>` with
a continue button *is* an open redirect, reachable without creating a link at
all, inside the feature added to prevent open redirects. This is how most
implementations of interstitials are written.

Therefore: served at `GET /{code}`, continuation at `GET /{code}?continue=1`.
The destination is re-read from the store on continuation and is **never**
accepted from the request. No parameter anywhere in this flow names a
destination.

The page shows the full destination as escaped text (the one place `Raw()` is
called for display), carries `no-store` and `no-referrer`, and loads **no
external resources** — a warning page that pulls third-party script to tell you
a third party is untrusted is both a contradiction and a destination leak.

**Done when**
- [ ] No handler in the package reads a URL from a query parameter — asserted by
      a test, not by review
- [ ] The default template contains no external URL — asserted by a test
- [ ] XSS attempt in the destination renders as text, in a case that has been
      seen to fail with the escaping removed
BODY

# ---- M7 --------------------------------------------------------------------

issue "Vanity codes: one keyspace, separated by length" "M7 Extras" "core,security" <<'BODY'
FR-11, SR-04, ADR-0011.

One keyspace, so resolution stays a single lookup and `SetNX` remains the
collision authority. Separation by **length**: a vanity code of exactly the
generated length is rejected with `ErrVanityLength`.

That one rule solves three problems at once — collision becomes structurally
impossible, squatting on future generated codes becomes impossible, and probing
the generated space becomes impossible because a generated-length candidate is
rejected on length **before any store lookup** and so returns nothing about
occupancy.

Plus a default reserved-word list (`admin`, `api`, `health`, `login`, `static`,
`assets`, `robots.txt`, `favicon.ico`, `.well-known`), extendable.

**Done when**
- [ ] A generated-length vanity code is rejected with zero store round trips —
      asserted with a counting fake
- [ ] Reserved words rejected with `ErrVanityReserved`
- [ ] `ErrCodeExists` is returned to the caller, never retried
- [ ] Godoc states a vanity code is a public name, never a capability
BODY

issue "OwnerLister with cursor pagination" "M7 Extras" "store" <<'BODY'
FR-10, ADR-0010. Cursor-paginated listing by owner, in `memstore` and
`redisstore`.

Index entries are removed by the same Lua script that writes them, so the index
does not grow past the records it points at.

`ListByOwner` is a **filter, not authorization** — passing an attacker's chosen
`ownerID` returns that owner's links, which is correct for a filter and a
vulnerability on an unauthenticated endpoint. The godoc says so.

**Done when**
- [ ] Pagination is stable across concurrent creation — no skipped or duplicated
      entries
- [ ] Collected records leave no index entry behind
- [ ] The godoc warning is present
BODY

issue "Counter interface, called on a detached context" "M7 Extras" "core,store" <<'BODY'
FR-14, ADR-0012. Segregated from `Store`, called after the redirect is on the
wire, best-effort, failures reported to `Hooks` and never to the visitor.

**The bug this issue exists to prevent:** using the request context. It is
cancelled the moment the response completes, so every count races the redirect it
is counting. It passes tests — a test that inspects the recorder after the
handler returns has already let the goroutine run — and then drops a variable
fraction of counts under real concurrency. Use `context.WithoutCancel` with its
own timeout.

**Done when**
- [ ] A test with an already-cancelled request context still records the count —
      negative control against the request-context version
- [ ] A `Count` error never changes the response
- [ ] `Close` drains pending counts for graceful shutdown; loss on a hard kill is
      documented, not hidden
BODY

issue "Deduplication: opt-in, off by default, scoped per owner" "M7 Extras" "core,security" <<'BODY'
FR-15, SR-17, ADR-0013.

Off by default because it is an existence oracle: submit a candidate URL and
learn whether someone else already shortened it, through the documented API with
no attack required.

When enabled it is scoped **per owner**, which keeps the storage saving that
motivated the feature and removes the cross-user disclosure entirely. An oracle
over an owner's own links is not a disclosure — they already know what they
shortened.

A dedup hit returns an existing link whose `ExpiresAt` may differ from what was
requested, and the requested TTL is **not** applied: silently extending a link's
life from a create call is an ownership violation.

**Done when**
- [ ] Off by default, asserted
- [ ] Two owners shortening the same URL get two codes
- [ ] The index stores `sha256`, never the URL — asserted against the keyspace
- [ ] A dedup hit does not modify the existing link's TTL
BODY

# ---- M8 --------------------------------------------------------------------

issue "Threat probe script covering T-01 to T-15" "M8 Verification" "testing,security" <<'BODY'
NFR-17, following `crier`'s `docs/security/probe-threats.sh` (IR-06 there).

One re-runnable script, pointed at the demo stack, with one probe per threat in
`docs/THREAT-MODEL.md`. Its output is recorded in the repository.

**A negative assertion is satisfied by a tool that never ran.** Every probe
asserts the *expected answer*, not the absence of a bad one, and resolves its
output into a variable so a non-zero exit is visible. Six checks in `crier`
passed without verifying anything before this was made a rule.

**Done when**
- [ ] Every `T-nn` has a probe, or an explicit line saying why it cannot be
      probed from outside
- [ ] Each probe has been seen to fail against a deliberately broken build
- [ ] The recorded output is committed
BODY

issue "Negative-control audit of every SR- test" "M8 Verification" "testing,security" <<'BODY'
The rule from `REQUIREMENTS.md` §1: *a requirement without a test that fails when
the protection is removed counts as unimplemented.*

Walk `SR-01` through `SR-24`. For each: locate the test, remove the protection,
watch it go red, restore, and record the control in a comment above the test.

Where no test exists, the requirement is unimplemented and gets an issue — not a
note.

**Done when**
- [ ] A table in `docs/audit-log.md` maps each `SR-` to its test and its verified
      control
- [ ] Every gap found became an issue
BODY

issue "Benchmarks, including the Destination unmask on the redirect path" "M8 Verification" "testing" <<'BODY'
ADR-0007 objection 3: building a `Location` header unmasks a `Destination`, which
costs an HMAC-SHA-256 keystream on the one path that must not become the
bottleneck. That objection was accepted on the argument that the cost is small —
this issue turns the argument into a number.

Also benchmark: code generation, alphabet validation, the full resolve path
against `memstore` and against a local Redis.

**Done when**
- [ ] The unmask cost is expressed as a percentage of redirect-path latency at
      realistic URL lengths — that is ADR-0007's reopening criterion
- [ ] Results are published in `docs/benchmarks.md`
BODY

issue "Runnable demo: docker-compose with Redis, moat and cairn" "M8 Verification" "docs,testing" <<'BODY'
IR-05. One command brings up Redis (AOF on, `noeviction`) and a minimal host
composing `moat` + `cairn` + `redisstore`, with create and resolve routes.

It is the target for the NFR-17 probes and the thing a reviewer runs without
reading code.

**Done when**
- [ ] `docker compose up` and a documented curl sequence create, resolve, revoke
- [ ] The Redis service is configured per ADR-0008's operational contract, so the
      demo demonstrates the contract rather than contradicting it
- [ ] The probe script runs green against it
BODY

# ---- M9 --------------------------------------------------------------------

issue "task-api integration guide, with the ownership negative test" "M9 Release" "docs,security" <<'BODY'
IR-04, `docs/INTEGRATION.md` §4.

The guide covers the Service-layer placement, the error envelope mapping, and the
410-for-authenticated / 404-for-anonymous decision (SR-03).

**The part that matters:** `Revoke` does not check ownership (ADR-0010). The
Service must load the link, compare `OwnerID` against the authenticated
principal, and only then revoke. This is the most likely integration bug in the
whole project, and it is the kind that passes every test written by the person
who wrote the code.

**Done when**
- [ ] `docs/integrations/task-api.md` exists, following `crier`'s shape
- [ ] `task-api` carries a test proving user A cannot revoke user B's link
- [ ] The guide states which cairn errors map to which envelope codes
BODY

issue "CHANGELOG and RELEASING" "M9 Release" "docs,release" <<'BODY'
NFR-13. `CHANGELOG.md` generated from the real API diff per release, not
hand-written. `RELEASING.md` documents the two tag prefixes (`vX.Y.Z` and
`redisstore/vX.Y.Z`) and which one triggers what.

**Done when**
- [ ] The tag pattern that triggers a release cannot match a test-only module
- [ ] The procedure includes verifying the published module from a clean
      directory outside the repository — the check `usher`'s ADR-11 established
      after a satellite tag shipped that did not compile
BODY

issue "Tag v0.1.0 and redisstore/v0.1.0" "M9 Release" "release" <<'BODY'
The acceptance criteria are `REQUIREMENTS.md` §8:

1. A host composes `moat` + `cairn` + `redisstore`, creates, resolves, revokes,
   and sees the revocation take effect on the next request with no cache to wait
   out.
2. Every `SR-` is covered by a test **seen to fail** with the protection removed.
3. `docs/adr/README.md` lists no open question the code has already answered by
   accident.
4. The NFR-17 probe output is recorded in the repository.

**Done when**
- [ ] Both modules resolve through the real proxy from a clean directory
- [ ] All four criteria are checked off with evidence, not assertion
BODY

echo

# --------------------------------------------------------------- project ----

if [[ $DRY_RUN -eq 1 ]]; then
  echo "[dry-run] project '$PROJECT_TITLE' + ${#ISSUE_URLS[@]} items"
  echo
  echo "Dry run complete."
  exit 0
fi

echo "Project board..."
PROJECT_NUMBER=$(gh project list --owner "$OWNER" --format json \
  --jq ".projects[] | select(.title==\"$PROJECT_TITLE\") | .number" 2>/dev/null | head -1 || true)

if [[ -z "${PROJECT_NUMBER:-}" ]]; then
  PROJECT_NUMBER=$(gh project create --owner "$OWNER" --title "$PROJECT_TITLE" \
    --format json --jq .number)
  echo "  created project #$PROJECT_NUMBER"
else
  echo "  using existing project #$PROJECT_NUMBER"
fi

echo "  adding ${#ISSUE_URLS[@]} issues..."
for url in "${ISSUE_URLS[@]}"; do
  gh project item-add "$PROJECT_NUMBER" --owner "$OWNER" --url "$url" >/dev/null
  printf '.'
done
echo
echo
echo "Done. ${#ISSUE_URLS[@]} issues in project #$PROJECT_NUMBER:"
echo "  https://github.com/users/$OWNER/projects/$PROJECT_NUMBER"
