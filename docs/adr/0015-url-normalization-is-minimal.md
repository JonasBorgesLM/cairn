# ADR-0015: URL normalization is minimal and semantics-preserving

## Status
Accepted

## Context
Normalization matters for two reasons: deduplication needs a canonical form
(ADR-0013), and a stored URL that differs from what the user submitted will
surprise them.

It is also easy to overreach. Every "harmless" normalization below breaks a real
server:

- **Sorting query parameters** — order is significant to APIs that accept
  repeated keys, and to any signature computed over the query string. Sorting
  a pre-signed URL invalidates it.
- **Stripping a trailing slash** — `/docs` and `/docs/` are different resources
  to plenty of servers, and one of them is usually a redirect.
- **Lowercasing the path** — case-sensitive on every filesystem that matters.
- **Removing `www.`** — a different hostname, which may resolve elsewhere.
- **Dropping the fragment** — the fragment is often the entire address in a
  single-page application.
- **Removing tracking parameters** (`utm_*`) — tempting, and it is editing
  someone's URL to remove information they chose to include.

## Decision
Normalize only what RFC 3986 §6.2.2 declares equivalent, and nothing else:

| Applied | Rule |
| --- | --- |
| ✅ | Scheme lowercased (§6.2.2.1) |
| ✅ | Host lowercased; IDN converted to punycode (§6.2.2.1) |
| ✅ | Default port removed — `:80` for http, `:443` for https (§6.2.3) |
| ✅ | Percent-encoding of unreserved characters decoded; hex digits uppercased (§6.2.2.1, §6.2.2.2) |
| ✅ | Empty path becomes `/` (§6.2.3) |
| ✅ | Dot segments resolved — `/a/./b/../c` → `/a/c` (§6.2.2.3) |
| ❌ | Query order, trailing slash, path case, `www.`, fragment, tracking parameters |

IDN → punycode is in the applied list for a security reason as well as a
canonical one: it makes the host comparable in ASCII, so the private-network
check (SR-07), the own-domain check (SR-09) and the allowlist (FR-13) all compare
the same bytes the resolver will. A homograph host that is compared as Unicode in
one place and punycode in another is a bypass.

Normalization runs **after** parse-time validation and **before** policy
evaluation, so policy sees the canonical form and never a variant that a check
would have to normalize itself.

`Destination.Raw()` returns the normalized form. cairn stores what it will
redirect to, and that is what the creator is shown at creation time — the
alternative, storing the original and redirecting to the normalized one, means
the API shows a URL the service does not use.

## Consequences
- Dedup misses URLs a human would call identical (`?a=1&b=2` versus `?b=2&a=1`).
  That is the safe direction: a false miss makes a redundant link; a false hit
  points a user at a destination they did not submit.
- The normalized URL may differ visibly from what was pasted, most often by a
  removed default port or a resolved dot segment. Documented, and small.
- A host wanting aggressive normalization — `utm_` stripping is the common
  request — does it before calling `Create`. That is their editorial decision
  about their users' URLs, and it belongs to them.
- Punycode conversion needs `golang.org/x/net/idna`, which is a third-party
  dependency the core may not take (ADR-0001). v1 therefore rejects hostnames
  containing non-ASCII rather than converting them, with `ReasonControlChars`
  extended to cover it. Rejecting is safe and honest; converting incorrectly is
  neither. Reopen if a consumer needs IDN, and the answer will be a satellite
  module, not a core dependency.
