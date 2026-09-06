# cairn demo

Issue #51, IR-05. A minimal host composing `moat` (security headers, rate
limiting), `cairn` (codes, policy, lifecycle) and `redisstore` (storage),
against a Redis configured per
[ADR-0008](../docs/adr/0008-store-contract-and-durability.md)'s operational
contract. This is the target for the threat probes of
[`docs/security/probe-threats.sh`](../docs/security/probe-threats.sh)
(NFR-17) and the thing to run before reading any code.

## What this is not

No authentication, no ownership check on revoke (ADR-0010 is explicit that
`cairn.Revoke` never checks it — a host exposing revocation enforces that
itself, and this demo has no accounts to enforce it against), no replica for
Redis (ADR-0008 wants one; a single `docker-compose` container cannot be one).
It demonstrates cairn's own contract, not a production deployment.

## Run it

```bash
docker compose up --build
```

From the repository root. Redis starts with `--appendonly yes
--appendfsync everysec --maxmemory-policy noeviction`
([ADR-0008](../docs/adr/0008-store-contract-and-durability.md)); the demo
refuses to start against anything else (`redisstore.EvictionCheck`,
`cairn.New`). Wait for `demo listening` in the logs, or poll
`curl -sf http://localhost:8080/nonexistent` until it returns rather than
connection-refuses.

## Create, resolve, revoke

**Create** a link:

```bash
curl -s -X POST http://localhost:8080/links \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com/docs"}'
```

```json
{"code":"AbCdEf1234","short_url":"http://localhost:8080/AbCdEf1234"}
```

**Resolve** it — a plain redirect, so `-i` to see the response rather than
follow it:

```bash
curl -si http://localhost:8080/AbCdEf1234
```

```
HTTP/1.1 302 Found
Cache-Control: no-store
Referrer-Policy: no-referrer
Location: https://example.com/docs
```

Never a 301 (SR-11), always `no-store` (SR-12) — follow it with `-L` to land
on the destination, or leave `-L` off to inspect the redirect itself.

**Revoke** it:

```bash
curl -si -X POST http://localhost:8080/links/AbCdEf1234/revoke
```

```
HTTP/1.1 204 No Content
```

Resolving again now returns `404`, indistinguishably from a code that never
existed unless the caller opts into telling them apart (SR-03):

```bash
curl -si http://localhost:8080/AbCdEf1234
curl -si http://localhost:8080/doesnotexist1
# both: HTTP/1.1 404 Not Found
```

## Things worth trying that should fail

```bash
# SR-05: scheme allowlist.
curl -s -X POST http://localhost:8080/links -d '{"url":"javascript:alert(1)"}'

# SR-07/T-05: internal destinations, including the metadata endpoint SSRF
# targets.
curl -s -X POST http://localhost:8080/links -d '{"url":"http://169.254.169.254/"}'

# SR-09/T-04: the service's own domain (anti-loop).
curl -s -X POST http://localhost:8080/links -d '{"url":"http://localhost:8080/AbCdEf1234"}'

# IR-01: the moat rate limiter, after burst (40) + sustained (20/s) is
# exceeded -- run this fast enough and a request gets a 429.
for i in $(seq 1 60); do curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/doesnotexist1; done
```

## Tear down

```bash
docker compose down -v
```
