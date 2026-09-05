# cairn — Threat Model

**Version:** 0.1
**Scope:** the `cairn` library and the short-link data it owns.
**Companion to:** [`../REQUIREMENTS.md`](../REQUIREMENTS.md) — every mitigation
below names the `SR-` it is discharged by, and every `SR-` appears at least once.

A threat model that lists only threats it defeats is marketing. §7 lists what
this design does **not** stop, and why.

---

## 1. What is being protected

| Asset | Why it matters | Exposure |
| --- | --- | --- |
| **The destination URL** | Carries tokens, session ids, PII in the query string; and is the thing an abuser wants to launder behind a reputable domain | Stored; logged; returned in `Location` |
| **The code → destination mapping** | Its integrity *is* the product. Repointing one code is the highest-impact compromise available | Stored; readable by anyone with the code |
| **The set of live codes** | Enumerating it exposes every destination anyone shortened, including private ones | Inferable by scanning |
| **Revocation** | A revoked link that still resolves is worse than never having offered revocation | Depends on write path + caching |
| **The reputation of the short domain** | Once the domain is on a phishing blocklist, every link the host ever issued is dead | Depends on destination policy |
| **The host's internal network** | Reachable through third parties that follow redirects | Depends on SR-07 |
| **Availability of resolution** | A dead resolve path breaks every link already in the wild — email, print, QR codes | Depends on store + retry bounds |

The asymmetry worth noting: **creation can fail loudly and recover; resolution
cannot.** A short link that has been printed on a poster has no second chance.
That asymmetry drives fail-closed on writes (SR-20) and the refusal to put a
cache in front of reads (SR-23).

---

## 2. Actors and their capabilities

| Actor | Assumed capability | Assumed *not* to have |
| --- | --- | --- |
| **Visitor** | Can request any code, any number of times, from many addresses | Credentials |
| **Scanner** | Visitor + automation + a botnet to defeat per-IP limits | Read access to the store |
| **Abuser (creator)** | An authenticated account of the host; can create links freely within the host's quota | Ability to bypass `Create` validation |
| **Curious insider** | Read access to application logs and metrics | Read access to Redis |
| **Store-adjacent attacker** | Can reach Redis (misconfigured network, shared instance) | — |
| **Third-party fetcher** | Not an attacker: a *victim*. A service that fetches URLs users submit and follows redirects | — |
| **Network observer** | Sees the short URL in Referer headers, proxy logs, TLS SNI | Plaintext body under TLS |

`cairn` assumes the host has already authenticated the creator. It has no
opinion about who may shorten (§1.2 of the requirements) — but it does assume
that `Create` is not an unauthenticated endpoint, and every mitigation that
leans on that says so.

---

## 3. Trust boundaries

```
        ┌──────────────────────── untrusted ────────────────────────┐
        │  Visitor / Scanner            Abuser (authenticated)      │
        └───────┬───────────────────────────────┬──────────────────┘
                │ GET /{code}                   │ POST /links
        ╔═══════▼═══════════════════════════════▼══════════════════╗
        ║  HOST — routing, authn/authz, TLS                        ║
        ║  moat: ratelimit · secureheaders · validate · realip     ║  ← IR-01
        ╠══════════════════════════════════════════════════════════╣
        ║  cairnhttp (optional)   SR-11 · SR-12 · SR-13 · SR-14    ║
        ╠══════════════════════════════════════════════════════════╣
        ║  cairn core                                              ║
        ║    policy      SR-05..SR-10   ← boundary A               ║
        ║    code gen    SR-01 · SR-02                             ║
        ║    lifecycle   SR-03 · SR-04                             ║
        ╠══════════════════════════════════════════════════════════╣
        ║  Store         SR-18..SR-23   ← boundary B               ║
        ╚═══════════════════════╤══════════════════════════════════╝
                                │
                        ┌───────▼────────┐
                        │  Redis         │  operational contract, ADR-0008
                        └────────────────┘
```

**Boundary A — the destination is attacker-controlled input**, even when the
creator is authenticated. Everything after `policy` may assume scheme, shape and
length are sane; nothing before it may.

**Boundary B — the code is attacker-controlled input on the read path.** SR-21
exists because this boundary is crossed by a string that will be concatenated
into a key. Validation happens on cairn's side of it, never the store's.

---

## 4. Threats

Each threat is `T-nn`, with impact and the requirements that discharge it.
"Residual" means what is left after the mitigation.

### T-01 — Code enumeration (scanning)

**Actor:** Scanner. **Impact:** disclosure of every destination shortened by
every user, including private ones. This is the highest-likelihood attack on any
shortener because it needs nothing but a loop.

**Historical note:** the 2016 study of a major provider's 6-character codes
enumerated a material fraction of the space and recovered documents and mapped
routes. Every one of those links was "unguessable" in the sense of not being
sequential. The lesson is exactly SR-01/SR-02: randomness was never the missing
property — density was.

**Mitigation:** SR-02 (density: `E[hits] = R · N / A^L`, default `L` chosen so
this stays negligible), SR-01 (no counter to walk), IR-01 (`moat` reduces `R`).

**Residual:** a distributed scanner defeats per-IP rate limiting, so density has
to hold on its own. It does — that is why SR-02 is stated as an inequality with
a default rather than as "we use random codes". A host storing far more links
than the default assumes must raise `L`, and the docs must give them the formula
to know when.

### T-02 — Sequential/predictable code inference

**Actor:** Scanner. **Impact:** targeted retrieval of a specific victim's link
(create one immediately after theirs, subtract).

**Mitigation:** SR-01 (`crypto/rand`, no counter, no timestamp, no hash of the
destination — a hash makes the code a function of a guessable input, which is
the same failure in a costume).

**Residual:** none known, conditional on the CSPRNG. A `CodeGenerator` supplied
by a consumer can reintroduce this; the interface documentation must say so in
the strongest terms available.

### T-03 — Link repointing

**Actor:** Abuser or Store-adjacent attacker. **Impact:** the worst case in this
system. A code already distributed — printed, emailed, embedded in a QR code —
begins resolving to the attacker's destination, with the host's reputation
attached.

**Mitigation:** SR-18 (conditional write only; there is no code path that
overwrites an existing key), SR-22 (namespaced keys, so a host's unrelated writes
cannot land on a cairn key by accident), immutability (a link's destination is
never edited — §7 of the requirements).

**Residual:** an attacker with direct write access to Redis repoints at will.
cairn cannot defend against that, and ADR-0008's operational contract states it
as an infrastructure requirement rather than leaving it implied.

### T-04 — Open redirect / phishing laundering

**Actor:** Abuser. **Impact:** the host's domain becomes the reputable wrapper on
a credential-harvesting page; the domain lands on blocklists; every link the host
ever issued dies with it.

**Mitigation:** SR-05 (scheme allowlist — closes `javascript:` and `data:`
outright), SR-06 (no userinfo — closes the
`https://accounts.google.com@evil.example/` display trick), SR-09 (anti-loop),
FR-13/SR-24 (interstitial for destinations outside a configured allowlist).

**Residual:** *substantial, and stated plainly.* An allowlist of schemes does not
make `https://evil.example/login` less of a phishing page. A general-purpose
shortener cannot decide whether an arbitrary destination is malicious. The
interstitial shifts the decision to the visitor; reputation feeds would shift it
further and are out of scope for v1. The honest statement is that cairn reduces
the *mechanical* laundering primitives and does not solve phishing.

### T-05 — SSRF by proxy (allowlist bypass for third parties)

**Actor:** Abuser. **Victim:** a third-party fetcher, not cairn. **Impact:** the
abuser shortens `http://169.254.169.254/latest/meta-data/iam/...`, submits the
short URL to a service whose allowlist trusts the short domain, and that service
follows the redirect into its own metadata endpoint.

**Mitigation:** SR-07 (reject loopback, RFC 1918, link-local, CGNAT, unspecified,
multicast, IPv6 ULA, IPv4-mapped IPv6, and non-decimal IPv4 literals rather than
attempting to parse them).

**Residual:** **TOCTOU on DNS.** The hostname is bound to an address at create
time; nothing stops it resolving elsewhere at resolve time. No create-time check
closes this. The complete mitigation belongs to the fetcher, which must validate
the address it actually connects to. SR-07 raises the attacker's cost and removes
the trivial literal-IP case; it does not remove the class, and the package
documentation says so where a reader will meet it.

### T-06 — Destination disclosure through logs

**Actor:** Curious insider; anyone downstream of the log pipeline. **Impact:** a
URL carrying a password-reset token, a signed S3 URL or a session id in its query
string, sitting in a log index with a long retention.

**Mitigation:** SR-15 — `Destination` redacts through every formatting path the
standard library offers, built on `moat`'s `secret.Value` (ADR-0007, IR-03), so
the accidental `%+v` and the `slog.Any("link", l)` both write a redacted form.
Extraction requires an explicit call that is visible in review.

**Residual:** a consumer who calls the explicit accessor and then logs the result
has defeated it. That is the intended trade: the primitive removes accidents, not
intent.

### T-07 — Revocation that does not revoke

**Actor:** any. **Impact:** revocation becomes advisory. A user who revoked a
link containing sensitive data believes it is gone.

**Mitigation:** SR-11 (never 301 — a cached permanent redirect is unrevocable in
every intermediary holding it), SR-12 (explicit `no-store`, because RFC 9111
§4.2.2 lets a 302 be heuristically cached anyway), SR-23 (no read cache in front
of the store in v1, so there is no stale copy to serve).

**Residual:** a browser holding an in-flight response, and any copy of the
destination the visitor already followed. Revocation stops future resolution; it
does not un-send what was already delivered, and the documentation must not imply
otherwise.

### T-08 — Existence oracle through deduplication

**Actor:** any creator. **Impact:** submitting a candidate URL and receiving an
existing code proves someone already shortened it — a disclosure about other
users' behaviour, using only the documented API.

**Mitigation:** SR-17 — deduplication is off by default, opt-in per host, and its
documentation states the disclosure rather than listing it as a feature
(ADR-0013).

**Residual:** a host that enables it accepts the oracle. That is their call to
make; cairn's obligation is to make sure it is a call and not an accident.

### T-09 — Vanity availability probing

**Actor:** any creator. **Impact:** mapping which vanity codes exist; squatting;
in the worst version, using vanity creation to probe the *generated* space.

**Mitigation:** SR-04 (a vanity code of the generated length is rejected, so the
generated space cannot be probed this way at all — this is the part that
matters), SR-16 (authentication and rate limiting at the host; constant response
shape), reserved-word list (ADR-0011).

**Residual:** the vanity namespace remains enumerable by an authenticated
creator. This is inherent to any vanity namespace — DNS has the same property —
and is accepted rather than papered over.

### T-10 — Denial of service on creation

**Actor:** Abuser. **Impact:** creation stalls; in the unbounded-retry version, a
saturated keyspace turns every create into a spin against the store.

**Mitigation:** SR-19 (bounded retry, hard ceiling, fail on exhaustion), IR-01
(`moat` rate limits creation), SR-02's density choice (a keyspace that is never
near saturation makes the retry path cold in the first place).

**Residual:** a host that configures a short `L` with a high `N` gets both a scan
problem and a retry problem from the same mistake. Validation at construction
(NFR-08) should refuse a configuration whose density is self-evidently unsafe,
and this is a design obligation recorded in ADR-0002, not a runtime warning.

### T-11 — Key injection through a hostile code

**Actor:** Visitor. **Impact:** a code containing `:` or `*` or a newline
addressing a key outside cairn's namespace, or colliding with a host key in a
shared Redis.

**Mitigation:** SR-21 (validate against the alphabet and the length bounds
*before* concatenating), SR-22 (fixed namespace prefix and schema version).

**Residual:** none, provided validation precedes key construction — which is why
the requirement is written about the *ordering*, not merely about validating.

### T-12 — Store unavailability treated as permission

**Actor:** none — this is a failure mode, and it is in the model because
fail-open is a decision that gets made by omission.

**Mitigation:** SR-20 (fail-closed on both paths, not configurable into
fail-open), and the `noeviction` startup verification of ADR-0008 (an eviction
policy that discards keys under pressure turns a memory spike into silent link
loss, and is the same failure `moat` guards against on its rate-limit store).

**Residual:** resolution is unavailable while the store is. That is the accepted
cost of not serving a stale or unvalidated redirect.

### T-13 — Referer leakage of the short code

**Actor:** the destination site, and every analytics script on it. **Impact:**
the destination learns the short code, which may itself be sensitive (it names a
specific recipient of a specific campaign) and lets the destination replay it.

**Mitigation:** SR-14 (`Referrer-Policy: no-referrer` on the redirect).

**Residual:** intermediaries that observed the request already saw the URL. TLS
covers the path, not the endpoints.

### T-14 — Interstitial as an open redirect

**Actor:** Abuser. **Impact:** an interstitial page built the obvious way —
`/warn?to=https://evil.example` with a continue button — *is* the open redirect
the interstitial was added to prevent, and it is reachable without creating a
link at all.

**Mitigation:** SR-24 — the continuation control references the short code and
nothing else. The destination is re-read from the store on continuation; it is
never accepted from the request.

**Residual:** none, given that constraint. It is listed because this is a
mitigation that reliably becomes a vulnerability during implementation, and a
threat model that does not name it will not stop it.

### T-15 — Total link loss from store durability

**Actor:** none — operational. **Impact:** every link ever issued dies at once,
permanently and irrecoverably, because Redis is the single source of truth.

**Mitigation:** ADR-0008's operational contract — AOF `everysec`, a replica, and
`noeviction`, verified at startup where it can be — published as part of the
library's contract rather than assumed.

**Residual:** real, and accepted for v1 as a documented operational requirement
rather than a layered store. ADR-0008 records what a layered design would have to
answer, so the decision is reopenable with the reasoning intact.

---

## 5. Coverage matrix

| Threat | Requirements |
| --- | --- |
| T-01 Enumeration | SR-02, SR-01, IR-01 |
| T-02 Predictable codes | SR-01, FR-07 |
| T-03 Repointing | SR-18, SR-22 |
| T-04 Phishing laundering | SR-05, SR-06, SR-09, FR-13, SR-24 |
| T-05 SSRF by proxy | SR-07, SR-10 |
| T-06 Log disclosure | SR-15, IR-03 |
| T-07 Failed revocation | SR-11, SR-12, SR-23 |
| T-08 Dedup oracle | SR-17 |
| T-09 Vanity probing | SR-04, SR-16 |
| T-10 Creation DoS | SR-19, SR-08, IR-01 |
| T-11 Key injection | SR-21, SR-22 |
| T-12 Fail-open | SR-20 |
| T-13 Referer leak | SR-14 |
| T-14 Interstitial redirect | SR-24 |
| T-15 Durability | ADR-0008, NFR-01 |

Every `SR-` appears above except **SR-03** (uniform rejection) and **SR-13**
(method restriction), which are hardening rather than answers to a single named
threat: SR-03 narrows the oracle in T-01 and T-07, and SR-13 removes the reason
to reach for a method-preserving redirect status in T-07.

---

## 6. Verification

Per NFR-17, this model is not finished as prose. Each `T-` becomes a probe in
`docs/security/probe-threats.sh`, following `crier`'s IR-06, run against the
IR-05 demo stack. Each `SR-` gets a unit test **validated by a negative
control** — the test is watched failing with the protection removed before it is
trusted passing with it in place. A check nobody has seen go red is not a check.

---

## 7. Explicitly not addressed

Listed so that nobody infers coverage from silence.

- **Malicious-but-well-formed destinations.** See T-04. No reputation feed, no
  content scanning, no blocklist in v1.
- **Chained third-party shorteners.** SR-09 covers the host's own domain only.
- **DNS rebinding between create and resolve.** See T-05.
- **An attacker with read or write access to the store.** Out of the library's
  reach; ADR-0008 states it as an infrastructure requirement.
- **Traffic analysis.** Who resolved which code, and when, is visible to the host
  and to anything on the path.
- **Abuse quotas per creator.** The host's, composed from `moat`.
- **Client-side link previews.** A messaging app that unfurls a short link
  resolves it before the visitor does; cairn cannot distinguish that fetch from a
  click, which is also why FR-14's counter is documented as best-effort and
  approximate rather than as an analytic.
