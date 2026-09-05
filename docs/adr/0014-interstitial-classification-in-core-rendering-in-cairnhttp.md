# ADR-0014: The core classifies; `cairnhttp` renders the interstitial

## Status
Accepted

## Context
An interstitial — "you are leaving example.com, this link goes to
`https://unknown.example/x`, continue?" — moves the trust decision to the visitor
for destinations the host has not allowlisted. It is the standard mitigation for
the residual half of T-04, which SR-05 and SR-06 do not touch: a well-formed
`https://` phishing page.

It also collides with a hard boundary: the core must not import `net/http` and
must not render HTML (§1.2 of the requirements, NFR-06).

And it has a failure mode that is worse than not having it. The obvious
implementation is a page at `/warn?to=<destination>` with a continue button. That
endpoint accepts a destination from the request and redirects to it — **it is an
open redirect**, reachable without creating a link at all, in the feature added
to prevent open redirects. This is not a hypothetical; it is how most
implementations of this feature are written.

## Decision
Split at the classification boundary.

**The core classifies.** `Policy.Evaluate` already returns three values
(ADR-0005). `policy.Allowlist(domains…)` returns `Interstitial` for
non-members. The decision is stored on the record at create time, so resolution
does not re-evaluate policy — which keeps DNS off the redirect path (ADR-0005)
and keeps the redirect's behaviour stable for the life of the link.

**`cairnhttp` renders.** `WithInterstitial(t *html.Template)` supplies the page.
Without it, an `Interstitial` link redirects normally — the feature is opt-in at
the transport, and its absence is not a silent failure to warn, because the
classification is still visible on the `Link` for a host rendering its own page.

**The continuation control references the code, never a URL (SR-24).** The
interstitial is served at the same path as the redirect — `GET /{code}` — with
continuation as `GET /{code}?continue=1` (or a POST to the same path). The
destination is re-read from the store on continuation and is never accepted from
the request. There is no request parameter anywhere in this flow that names a
destination, so the open-redirect variant is not reachable by construction rather
than by validation.

The rendered page:
- shows the **full destination** — the whole point is that the visitor sees where
  they are going, so this is the one place `Destination.Raw()` is called for
  display, and the template escapes it as text, never as an `href` attribute of
  an auto-following element;
- carries `Cache-Control: no-store` and `Referrer-Policy: no-referrer`, the same
  as the redirect (ADR-0006), so an interstitial does not become the cached
  artifact the redirect refused to be;
- carries no external resources: no CDN, no analytics, no remote font. A warning
  page that loads third-party script to tell you a third party is untrusted is a
  contradiction, and it is also the thing that will leak the destination.

## Consequences
- Classification is fixed at creation. A destination added to the allowlist later
  keeps warning until the link is recreated. Accepted: re-evaluating on resolve
  costs a DNS lookup and a policy call on the redirect path, and makes a link's
  behaviour depend on configuration state at click time.
- The default template is deliberately plain and unstyled. A host will replace
  it, and shipping something that looks finished discourages that.
- `Interstitial` links are still stored, resolved and counted normally. The
  classification changes what the visitor sees, not what the link is.
- The bot problem is acknowledged and not solved: a crawler or a link-preview
  fetcher does not click "continue", so an interstitial makes previews break.
  That is the cost of the feature and a reason it is opt-in.
