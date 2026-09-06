#!/usr/bin/env bash
# Exercises docs/THREAT-MODEL.md's T-01 through T-15 against a running demo
# (NFR-17, following crier's docs/security/probe-threats.sh, IR-06 there).
#
# It is a script rather than a Go test on purpose: a security exercise should
# be re-runnable by someone who does not want to read the test suite first,
# and every request below is one they can paste individually.
#
#   docker compose up --build -d
#   docs/security/probe-threats.sh
#
# Exit status is the number of probes that did not get the expected answer.
#
# Negative controls actually run against this script (not left as a claim):
#   - T-15: `redis-cli CONFIG SET maxmemory-policy allkeys-lru` against the
#     running container, checked in isolation (not through this script, whose
#     own T-12 probe restarts Redis and would silently reset it back to the
#     compose file's noeviction flag first). Failed as expected, reporting
#     the live allkeys-lru value. Restored with CONFIG SET back to noeviction.
#   - T-05: demo/main.go's policy.Default(...) temporarily replaced with an
#     always-Allow policy, image rebuilt. T-05 failed (got 201, want 422) as
#     expected. T-04 correctly kept passing through the same change: SR-05's
#     scheme allowlist lives in ParseDestination, not Policy (ADR-0005's
#     stated division), so a broken Policy cannot touch it -- confirming the
#     architecture rather than the probe. Restored and rebuilt.
#   - Every other probe here observes, from outside, a protection that
#     already has its own negative control run and recorded during the M8
#     SR- audit (issue #49, docs/audit-log.md) at the unit level, which
#     isolates the exact line rather than guessing from an HTTP response.
#     Re-breaking cairn's pinned dependency to re-verify the same fact through
#     this script would need a local `replace` directive in demo/go.mod for
#     no additional confidence; not done for that reason.
set -uo pipefail

DEMO="${CAIRN_DEMO:-http://localhost:8080}"

failures=0
probe() { printf '%-58s ' "$1"; }
pass()  { echo "PASS  ($1)"; }
fail()  { echo "FAIL  ($1)"; failures=$((failures + 1)); }
note()  { echo "----  $1"; }

status() { curl -sg -o /dev/null -w '%{http_code}' "$@"; }
body()   { curl -sg "$@"; }

# curl reports 000 when it never spoke HTTP -- connection refused, DNS
# failure, a target that is not running. Several probes below assert "not
# 302"/"not 201", and 000 satisfies that trivially, so without this a probe
# can pass against nothing at all (the exact failure crier's own script
# documents having shipped once).
answered() {
  case "$1" in
    ""|000) return 1 ;;
    *) return 0 ;;
  esac
}

require_reachable() {
  local code
  code=$(status "$DEMO/doesnotexist1")
  if ! answered "$code"; then
    echo "the demo is not answering at $DEMO (curl reported '$code')." >&2
    echo "Start it first:  docker compose up --build -d" >&2
    exit 1
  fi
}
require_reachable

create() {
  # Prints the code, or nothing if create was rejected. $1 is the URL.
  body -X POST "$DEMO/links" -H 'Content-Type: application/json' \
    -d "{\"url\":\"$1\"}" | grep -o '"code":"[^"]*"' | cut -d'"' -f4
}

# --- T-01: code enumeration (scanning) --------------------------------------
# SR-02's density bound cannot be exercised by a handful of requests; what
# this script can check externally is the visible half of SR-01: codes are
# not sequential or otherwise derivable from a previous one.
probe "T-01: consecutive codes are not sequential"
c1=$(create "https://example.net/t01-a")
c2=$(create "https://example.net/t01-b")
if [ -z "$c1" ] || [ -z "$c2" ]; then
  fail "create did not return a code"
elif [ "$c1" = "$c2" ]; then
  fail "two distinct creates returned the same code"
else
  # A counter or timestamp source tends to produce one code as a literal
  # prefix of the next; crypto/rand output does not.
  if [ "$c1" = "${c2:0:${#c1}}" ] || [ "$c2" = "${c1:0:${#c2}}" ]; then
    fail "one code is a prefix of the other"
  else
    pass "$c1 / $c2, no shared prefix relationship"
  fi
fi

# --- T-02: sequential/predictable code inference ----------------------------
probe "T-02: codes are fixed-length from the configured alphabet"
if [ "${#c1}" -eq "${#c2}" ] && [ "${#c1}" -gt 0 ]; then
  pass "both length ${#c1}"
else
  fail "lengths differ or empty: ${#c1} vs ${#c2}"
fi

# --- T-03: link repointing ---------------------------------------------------
note "T-03: not probed externally. This demo's create route never lets a"
note "  caller choose a code (no vanity exposed), so there is no HTTP-reachable"
note "  way to attempt a second Save at an existing code. SR-18's conditional"
note "  write is covered by TestSave_SecondSaveLeavesOriginalByteIdentical's"
note "  negative control (docs/audit-log.md) instead."

# --- T-04: open redirect / phishing laundering ------------------------------
probe "T-04: disallowed scheme is rejected (SR-05)"
code=$(status -X POST "$DEMO/links" -H 'Content-Type: application/json' -d '{"url":"javascript:alert(1)"}')
[ "$code" = "422" ] && pass "$code" || fail "got $code, want 422"

note "T-04: the interstitial half (FR-13/SR-24) is not probed here --"
note "  policy.Default never returns Interstitial, so this demo's policy has"
note "  no path that reaches it. Covered by TestHandler_ContinueIgnoresAny..."
note "  and the negative control recorded in docs/audit-log.md's SR-24 row."

# --- T-05: SSRF by proxy -----------------------------------------------------
probe "T-05: internal/metadata destination is rejected (SR-07)"
code=$(status -X POST "$DEMO/links" -H 'Content-Type: application/json' -d '{"url":"http://169.254.169.254/latest/meta-data/"}')
[ "$code" = "422" ] && pass "$code" || fail "got $code, want 422"

# --- T-06: destination disclosure through logs ------------------------------
probe "T-06: the raw destination never reaches the demo's own logs"
marker="probe-t06-$(date +%s)-supersecrettoken"
code=$(create "https://example.net/download?token=$marker")
if [ -z "$code" ]; then
  fail "create was rejected, nothing to check"
else
  status "$DEMO/$code" >/dev/null
  if docker compose logs demo 2>/dev/null | grep -q "$marker"; then
    fail "the token appeared in the demo container's own logs"
  else
    pass "token absent from demo logs (SR-15 holding end to end)"
  fi
fi

# --- T-07: revocation that does not revoke ----------------------------------
probe "T-07: revoked link stops redirecting immediately"
code=$(create "https://example.net/t07")
if [ -z "$code" ]; then
  fail "create was rejected"
else
  before=$(status "$DEMO/$code")
  status -X POST "$DEMO/links/$code/revoke" >/dev/null
  after=$(status "$DEMO/$code")
  if [ "$before" = "302" ] && [ "$after" != "302" ]; then
    pass "302 -> $after"
  else
    fail "before=$before after=$after, want 302 -> non-302"
  fi
fi

probe "T-07: redirect never carries a caching directive that survives it"
code=$(create "https://example.net/t07b")
cc=$(body -si "$DEMO/$code" | grep -i '^Cache-Control:' | tr -d '\r')
if echo "$cc" | grep -qi 'no-store'; then
  pass "$cc"
else
  fail "Cache-Control = '$cc', want no-store"
fi

# --- T-08: existence oracle through deduplication ---------------------------
probe "T-08: dedup is off by default (same URL twice -> two codes)"
u="https://example.net/t08-$(date +%s)"
a=$(create "$u")
b=$(create "$u")
if [ -n "$a" ] && [ -n "$b" ] && [ "$a" != "$b" ]; then
  pass "$a / $b"
else
  fail "a='$a' b='$b', want two distinct non-empty codes"
fi

# --- T-09: vanity availability probing --------------------------------------
note "T-09: not probed. This demo's POST /links accepts only a destination"
note "  URL, never a caller-chosen code, so vanity creation is not reachable"
note "  through it at all. Covered by TestCreate_VanityCode_* (create_test.go)."

# --- T-10: denial of service on creation ------------------------------------
probe "T-10: a creation burst is rate limited (IR-01)"
codes=$(seq 1 80 | xargs -P 16 -I{} \
  curl -s -o /dev/null -w '%{http_code}\n' -X POST "$DEMO/links" \
    -H 'Content-Type: application/json' -d '{"url":"https://example.net/t10"}')
limited=$(echo "$codes" | grep -c '^429$' || true)
[ "$limited" -gt 0 ] && pass "$limited of 80 refused" || fail "nothing was rate limited"
# The bucket this just emptied is shared by every route behind the preset
# chain (T-10 targets /links, but the limiter keys on client address, not
# path), so the probes after this one need it to refill rather than reading
# their own 429s as a finding about T-11 through T-15.
sleep 3

# --- T-11: key injection through a hostile code -----------------------------
probe "T-11: a code with store-special characters is just not-found"
code=$(status "$DEMO/abc:*[def]")
[ "$code" = "404" ] && pass "$code" || fail "got $code, want 404 (not 500, not a Redis error)"

# --- T-12: store unavailability treated as permission -----------------------
probe "T-12: create and resolve fail closed with Redis stopped"
existing=$(create "https://example.net/t12-before-outage")
docker compose stop redis >/dev/null 2>&1
sleep 1
create_code=$(status -X POST "$DEMO/links" -H 'Content-Type: application/json' -d '{"url":"https://example.net/t12"}')
resolve_code=$(status "$DEMO/${existing:-abc1234567}")
docker compose start redis >/dev/null 2>&1
# Wait for the healthcheck rather than a fixed sleep: ADR-0008's noeviction
# check runs on the next Store call, not at container start, so the demo
# process itself needs no restart -- only Redis needs to be reachable again.
for _ in $(seq 1 15); do
  docker compose exec -T redis redis-cli ping >/dev/null 2>&1 && break
  sleep 1
done
if [ "$create_code" = "503" ] && [ "$resolve_code" = "503" ]; then
  pass "create=503 resolve=503"
else
  fail "create=$create_code resolve=$resolve_code, want both 503 (SR-20)"
fi

# --- T-13: Referer leakage of the short code --------------------------------
probe "T-13: redirect carries Referrer-Policy: no-referrer"
code=$(create "https://example.net/t13")
rp=$(body -si "$DEMO/$code" | grep -i '^Referrer-Policy:' | tr -d '\r')
echo "$rp" | grep -qi 'no-referrer' && pass "$rp" || fail "Referrer-Policy = '$rp'"

# --- T-14: interstitial as an open redirect ---------------------------------
note "T-14: not probed. Same reason as T-04's interstitial half: this demo's"
note "  policy never returns Interstitial. Covered by"
note "  TestHandler_ContinueIgnoresAnyDestinationSuppliedInTheRequest and"
note "  TestHandler_NeverReflectsAQueryParameterAsADestination, both with a"
note "  recorded negative control (docs/audit-log.md, SR-24)."

# --- T-15: total link loss from store durability ----------------------------
probe "T-15: Redis is configured per ADR-0008 (AOF everysec, noeviction)"
cfg=$(docker compose exec -T redis redis-cli CONFIG GET appendonly 2>/dev/null | tail -1)
sync=$(docker compose exec -T redis redis-cli CONFIG GET appendfsync 2>/dev/null | tail -1)
policy=$(docker compose exec -T redis redis-cli CONFIG GET maxmemory-policy 2>/dev/null | tail -1)
if [ "$cfg" = "yes" ] && [ "$sync" = "everysec" ] && [ "$policy" = "noeviction" ]; then
  pass "appendonly=yes appendfsync=everysec maxmemory-policy=noeviction"
else
  fail "appendonly=$cfg appendfsync=$sync maxmemory-policy=$policy"
fi
note "T-15: the demo already enforces the noeviction half at every startup"
note "  (redisstore.EvictionCheck, called from cairn.New) -- this probe is a"
note "  second, independent check of the same fact plus the AOF settings"
note "  EvictionCheck does not look at. A replica and an exercised restore"
note "  procedure, ADR-0008's other two requirements, are not demonstrated by"
note "  a one-container compose file; see demo/README.md."

echo
echo "probes failed: $failures"
exit "$failures"
