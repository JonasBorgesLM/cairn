# SR- negative-control audit

Issue #49. Per `REQUIREMENTS.md` §1: a requirement without a test that fails
when the protection is removed counts as unimplemented. This walks `SR-01`
through `SR-24`, names the test that guards each, and records the control that
was actually run against it during this audit (2026-09-06) — protection
removed, test watched failing, protection restored — except where a control
was already recorded during the milestone that implemented the requirement,
in which case that comment is the record and is cited here rather than
repeated.

A "restored" entry below means the working tree was diffed back to identical
after the control; none of these controls left a trace in the shipped code.

| SR | Requirement | Test | Control |
| --- | --- | --- | --- |
| SR-01 | Unpredictability (CSPRNG, rejection sampling) | `TestNewRandomGenerator_ChiSquareUniformity` (code_test.go) | Recorded at implementation (M2): rejection check disabled, chi-square statistic 459.50 against the alphabet's 61-degree-of-freedom threshold. Restored. |
| SR-02 | Scan resistance (keyspace density) | `TestNew_RejectsUnsafeKeyspaceDensity` (shortener_test.go) | Run this audit: `CheckKeyspaceDensity` call removed from `New`'s default-generator branch. Failed — error was nil. Restored. |
| SR-03 | Uniform rejection (not-found/expired/revoked indistinguishable by default) | `TestDefaultErrorEncoder_UniformRejection` (cairnhttp/errors_test.go) | **Defect found and fixed this audit** (issue #73): `DefaultErrorEncoder` distinguished 404/410 by default, the inverse of what SR-03 requires. Run against the pre-fix M6 build: failed (410 for revoked/expired vs 404 for not-found). Fixed forward; see the commit closing #73. |
| SR-04 | Vanity never occupies the generated length | `TestNew_RejectsVanityRangeOverlappingGeneratedLength` (shortener_test.go) | Run this audit: overlap check removed from `New`. Failed — error was nil. Restored. |
| SR-05 | Scheme allowlist | `TestParseDestination_RejectsSchemesOutsideAllowlist` (destination_test.go) | Recorded at implementation (M1): scheme check removed, `javascript:` accepted. Restored. |
| SR-06 | No embedded credentials | `TestParseDestination_RejectsUserinfo` (destination_test.go) | Recorded at implementation (M1): userinfo check removed, credentials accepted. Restored. |
| SR-07 | No internal destinations | `TestBlockPrivateNetworks_RejectsNonDecimalIPv4Forms` (policy/privatenetworks_test.go) | Recorded at implementation (M3): ambiguous-literal check disabled, non-decimal IPv4 forms fell through to DNS resolution instead of being rejected outright. Restored. |
| SR-08 | Destination length limit (2000 bytes, checked before parsing) | `TestParseDestination_RejectsTooLong` (destination_test.go) | Run this audit: length check removed from `ParseDestination`. Failed — error was nil. Restored. |
| SR-09 | Anti-loop (reject the service's own domain) | `TestDefault_DeniesTheServicesOwnDomain` (policy/default_test.go) | Run this audit: `ownDomains.Evaluate` forced to always return Allow. Failed — request fell through to `BlockPrivateNetworks` and was denied for the wrong reason (`host_resolution_failed`, not `own_domain`). Restored. |
| SR-10 | No control characters or whitespace | `TestParseDestination_RejectsCRLFInjection` (destination_test.go) | Run this audit: raw-level `containsControlOrWhitespace` check removed from `ParseDestination`. Failed — `RejectReasonFrom` reported `("", false)`. Restored. |
| SR-11 | Never 301 | `TestHandler_RedirectsWithFoundStatus` + `TestPackage_NeverReferencesStatusMovedPermanently` (cairnhttp/handler_test.go) | Run this audit: `ServeHTTP`'s `WriteHeader` call changed to `http.StatusMovedPermanently`. Both tests failed — status 301 vs 302, and the literal-reference scan found it. Restored. |
| SR-12 | Explicit `no-store` | `TestHandler_CacheControlSurvivesWithoutExplicitNoStore` (cairnhttp/handler_test.go) | Recorded at implementation (M6): `Cache-Control: no-store` line removed, header absent. Restored. |
| SR-13 | GET and HEAD only | `TestHandler_RejectsNonGetHeadMethods` (cairnhttp/handler_test.go) | Run this audit: method check removed from `ServeHTTP`. Failed — every non-GET/HEAD method still got a 302. Restored. |
| SR-14 | No Referer leak (`Referrer-Policy: no-referrer`) | `TestHandler_SetsReferrerPolicyNoReferrer` (cairnhttp/handler_test.go) | Run this audit: `Referrer-Policy` header line removed from `ServeHTTP`. Failed — header was empty. Restored. |
| SR-15 | Destinations redact by default | `TestDestination_RedactsThroughEveryFormattingPath` (destination_test.go) | Recorded at implementation (M1): redaction path disabled, secret token appeared in output. Restored. |
| SR-16 | Vanity availability is an oracle, treated as one | — | No dedicated negative-control test. SR-16's cairn-side content is fully discharged by SR-04's tested exclusion (a vanity code can never probe the generated space) and by `RejectReason`'s uniform, synchronous, typed-error shape — there is no cairn-internal code path whose accidental omission would introduce response-shape variance between "taken" and "reserved". The remaining mitigations SR-16 lists (authentication, rate limiting via `moat`) are IR-01, the host's own responsibility, documented rather than implemented here (see `docs/THREAT-MODEL.md` T-09). Not an issue — there is nothing to implement inside cairn. |
| SR-17 | Deduplication off by default | `TestCreate_DedupIsOffByDefault` (create_test.go) | Run this audit: `s.dedup` gate on the lookup step forced to `true`. Failed — nil-pointer panic on `s.destIndex` (`New` only sets it when dedup is enabled), demonstrating that an unconditional dedup lookup breaks exactly the callers who never opted in. Restored. |
| SR-18 | Conditional write only (`SETNX`) | `TestSave_SecondSaveLeavesOriginalByteIdentical` (redisstore/integration_save_test.go) | Recorded at implementation (M5): `save.lua`'s `EXISTS` check skipped (unconditional `HSET`), the attacker's destination overwrote the original. Restored. |
| SR-19 | Bounded retry | `TestCreate_RetryTerminatesPromptlyRatherThanLooping` (create_test.go) | Recorded at implementation (M1/M2): retry loop bound changed to unconditional, loop did not terminate within the test's timeout. Restored. |
| SR-20 | Fail-closed | `TestCreate_StoreErrorYieldsNoLinkAndNoSuccessHook` (create_test.go); `TestBlockPrivateNetworks_ResolverErrorProducesDenyNeverAllow` (policy/privatenetworks_test.go) | Create-path control run this audit: `createGenerated`'s `!errors.Is(err, ErrCodeExists)` early return removed, so any Save error retried like a collision. Failed — error surfaced as `ErrCodeSpaceExhausted` after exhausting retries, not `ErrStoreUnavailable`. Restored. Policy-path control recorded at implementation (M3): resolver-error branch returned Allow, failed as expected. |
| SR-21 | Validate the code before building the key | `TestResolve_InvalidCodePerformsZeroStoreRoundTrips` (resolve_test.go) | Recorded at implementation (M1): order reversed (`Store.Load` before `Alphabet.Validate`), `loadCallCount` was 1, not 0. Restored. |
| SR-22 | Namespaced, versioned keys | `TestLinkKey_IsNamespacedAndVersioned`, `TestOwnerKey_IsNamespacedAndVersioned`, `TestHitsKey_IsNamespacedAndVersioned` (redisstore/keys_test.go) | Run this audit: `keySchema` emptied. All three failed — e.g. `linkKey` produced `":link:abc1234567"` instead of `"cairn:v1:link:abc1234567"`. Restored. |
| SR-23 | Revocation beats caching, by construction | `TestResolve_ReflectsRevocationOnTheVeryNextCall` (resolve_test.go) | Run this audit: a naive `map[Code]*Link` memoization cache added to `Shortener` and consulted at the top of `Resolve`, bypassing the revoked/expired re-check on a hit. Failed — the second `Resolve` returned the link successfully instead of `ErrLinkRevoked`. Cache removed immediately after; `git diff` confirmed byte-identical to the committed `cairn.go`/`resolve.go`. |
| SR-24 | Interstitial continuation references the code, never a URL | `TestHandler_ContinueIgnoresAnyDestinationSuppliedInTheRequest`, `TestHandler_NeverReflectsAQueryParameterAsADestination` (cairnhttp/interstitial_test.go) | Run this audit: `ServeHTTP` changed to read a `to` query parameter and substitute it for the stored destination when present — the literal open redirect ADR-0014 exists to prevent. Failed — `Location` became `https://evil.example/`. Restored. |

## Gaps found

One: **SR-03** (see table). Filed as issue #73, fixed in the same PR as this
audit. No other gap required a new issue — every other `SR-` already had, or
now has, a test that has been watched failing with its protection removed.

## Scope note

This audit covers `SR-01` through `SR-24` as listed in `REQUIREMENTS.md` §4.
It does not re-run the threat probes of `docs/THREAT-MODEL.md` §6 (issue #48,
covered separately) or the benchmark work of issue #50.
