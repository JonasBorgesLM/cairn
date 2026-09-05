# ADR-0006: 302 with explicit no-store; never 301

## Status
Accepted

## Context
The status code choice for a shortener is usually presented as 301 versus 302,
with 301 winning on the grounds that it is "the semantically correct permanent
redirect" and is better for SEO.

It is the wrong trade for a link that can be revoked. A 301 is cacheable by
default, indefinitely, in every browser and intermediary that receives it. Once
one is cached, revocation is an illusion: the visitor never asks again, and the
service has no way to reach the copy. A user who revokes a link containing a
password-reset token and is told it is revoked has been misled (T-07).

## Decision
`cairnhttp` answers **`302 Found`**, and — this is the half that is usually
missed — sets **`Cache-Control: no-store`** explicitly.

302 alone is not sufficient. RFC 9111 §4.2.2 permits a cache to apply a
heuristic freshness lifetime to a response with no explicit expiry, and 302 is
not on the list of statuses barred from that. A 302 without directives can
therefore be held for a while by a well-behaved cache. `no-store` closes it.

Also on the redirect response:

- `Referrer-Policy: no-referrer` (SR-14) — otherwise the destination and every
  script on it learns the short code, which frequently identifies the recipient
  of a specific campaign and can be replayed.
- `Pragma: no-cache` for HTTP/1.0-era intermediaries. Obsolete, harmless, and
  the intermediaries in question are exactly the ones that will not be upgraded.

**Non-GET methods get `405`** (SR-13) rather than a redirect. That is what
removes the reason to consider 307/308: those statuses exist to preserve the
method across a redirect, and a shortener that never redirects a non-GET has no
method to preserve. 307 would also be *worse* here in a subtle way — it invites
a client to replay a request body to a destination the shortener does not
control.

SEO is not a consideration. cairn is a library for application links, not a
marketing redirect service, and no amount of link equity is worth an unrevocable
redirect.

## Consequences
- Every resolution is a store round trip. That is the intended cost of SR-23:
  there is nothing between the store and the visitor that could serve a revoked
  link.
- Resolution throughput is bounded by the store. The mitigation is a fast store
  and horizontal scaling, not caching — and if caching is ever added, ADR-0008's
  reopening criterion applies and this ADR gets an amendment, not a quiet edit.
- A consumer who genuinely wants permanent, uncacheable-by-design links has to
  say so at the HTTP layer themselves. `cairnhttp` offers no option to emit 301;
  an option to break revocation is not an option, it is a trap.
