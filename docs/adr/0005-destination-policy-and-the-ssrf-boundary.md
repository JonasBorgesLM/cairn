# ADR-0005: Destination policy is required, composable, and honest about SSRF

## Status
Accepted

## Context
A shortener that accepts any destination is an open redirect with a database.
The requirements name six checks (SR-05 through SR-10). Three questions follow:
where the checks live, whether they can be skipped, and how honestly the
network-level check is described.

## Decision

### The policy is not optional
`cairn.New` returns an error when no `Policy` is configured. There is no default
policy and no permissive fallback. A shortener without destination policy is the
thing this library exists not to be, so the unsafe configuration is made
unrepresentable rather than merely discouraged.

This falls out of the package layout — `policy` imports `cairn`, so `cairn`
cannot import `policy` to default to it — and the constraint is welcome rather
than tolerated. The cost is one line in every consumer's setup.

### Syntax in the core, semantics in `policy/`
`ParseDestination` (core) does the checks that need no context and no network:
length before parsing (SR-08), scheme allowlist (SR-05), userinfo rejection
(SR-06), control characters and whitespace (SR-10). These are properties of the
string and belong with the type.

`Policy.Evaluate` (implementations in `policy/`) does the checks that need a
context: address classification (SR-07), own-domain (SR-09), allowlist
membership (FR-13). It receives a `context.Context` because it may resolve names.

### Three-valued decision
`Deny` / `Interstitial` / `Allow`. `Chain` composes with first-non-`Allow`-wins
and `Deny` beating `Interstitial`, so no ordering of policies can downgrade a
rejection into a warning.

A `Policy` that cannot reach its resolver returns `Deny` with
`ReasonResolveFailed`. Failing open here would make DNS unavailability a
temporary permission to shorten internal addresses (SR-20).

### The blocked set (SR-07)
Loopback (`127.0.0.0/8`, `::1`), RFC 1918, link-local (`169.254.0.0/16`
including `169.254.169.254`, `fe80::/10`), CGNAT (`100.64.0.0/10`), unspecified
(`0.0.0.0/8`, `::`), multicast, IPv6 unique-local (`fc00::/7`), and
IPv4-mapped IPv6 (`::ffff:0:0/96`) — the last because `http://[::ffff:127.0.0.1]/`
is loopback wearing a different notation, and a checker that only knows IPv4
literals waves it through.

Non-decimal IPv4 literals — `0x7f.0.0.1`, `2130706433`, `0177.0.0.1` — are
**rejected outright rather than parsed and classified**. Every parser disagrees
about them, and the disagreement between cairn's parser and the victim's is
precisely the bypass. Refusing a form nobody legitimately writes costs nothing.

Both the hostname's resolved addresses and any literal address in the host are
checked. `Resolver` is an interface so SR-07 is testable without a network — a
test that needs DNS is a test that will be skipped.

### What SR-07 does not do, stated where it will be read
cairn never fetches a destination, so this check protects nothing of cairn's. It
protects **third parties that follow redirects**: without it, an abuser turns an
allowlisted short domain into a bypass into someone else's metadata endpoint
(T-05).

And it binds a hostname to an address **at create time**. DNS may answer
differently at resolve time. No create-time check can close that, and the
complete mitigation belongs to the fetching service, which must validate the
address it actually connects to. This paragraph goes in the package
documentation, not only in this ADR, because a reader who meets
`BlockPrivateNetworks` in an IDE autocomplete will otherwise assume it is
complete.

## Consequences
- Creating a link may perform a DNS lookup, so `Create` is slower and can fail
  for reasons unrelated to the caller. Acceptable: creation is the path that is
  allowed to fail (see the asymmetry in the threat model, §1).
- A destination that was safe at creation can become unsafe later. cairn does not
  re-validate on resolve; doing so would put a DNS lookup on the redirect path
  and make resolution depend on a resolver's availability, which fails the
  asymmetry in the other direction.
- `policy.Allowlist` returning `Interstitial` rather than `Deny` is what makes
  ADR-0014 possible without a second classification pass.
