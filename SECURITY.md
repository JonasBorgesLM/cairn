# Security Policy

## Status

`cairn` is pre-implementation and pre-1.0. There is no released version, so there
is nothing deployed to be vulnerable yet. This policy is in place from the start
so that it exists before it is needed.

## Reporting a vulnerability

Use GitHub's [private vulnerability
reporting](https://github.com/JonasBorgesLM/cairn/security/advisories/new) on
this repository. Do not open a public issue.

Please include: the requirement or threat id it defeats if you know it
(`SR-nn` / `T-nn`), a minimal reproduction, and the impact you believe it has.

Expect an acknowledgement within 7 days. This is a personal project, not a
funded one, and that is set as an honest expectation rather than an SLA.

## Scope

**In scope** — anything that breaks a stated security requirement:

- A code that can be predicted or enumerated more cheaply than
  [`REQUIREMENTS.md`](REQUIREMENTS.md) SR-01/SR-02 claim.
- Any path that overwrites an existing code (SR-18).
- Any path that produces a redirect from an unvalidated or revoked link
  (SR-11, SR-20, SR-23).
- Any formatting, logging or marshalling route that emits an unredacted
  destination (SR-15).
- A destination that passes policy and reaches a blocked address class (SR-07).
- A code that reaches key construction without alphabet validation (SR-21).
- An open redirect anywhere in `cairnhttp`, and in the interstitial especially
  (SR-24).

**Out of scope** — documented residuals, listed in
[`docs/THREAT-MODEL.md`](docs/THREAT-MODEL.md) §7:

- That cairn does not detect malicious-but-well-formed phishing destinations.
- DNS rebinding between create and resolve (T-05 residual) — unclosable at
  create time; it belongs to the service that follows the redirect.
- That an attacker with read or write access to the store can do what that
  access implies.
- That vanity code availability is an oracle to an authenticated creator (T-09).
- That deduplication, when a host enables it, is an existence oracle (T-08).

If you think a documented residual is understated rather than accepted, that is
worth reporting — say which paragraph and why.

## Supported versions

None yet. From `v0.1.0`, the latest minor of the current major receives fixes.
