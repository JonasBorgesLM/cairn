-- save.lua atomically creates a link record and its owner-index entry.
--
-- KEYS[1] = link key   (HASH)
-- KEYS[2] = owner key  (ZSET)
-- ARGV[1] = code
-- ARGV[2] = owner_id
-- ARGV[3] = dest_raw
-- ARGV[4] = vanity        ("1" or "0")
-- ARGV[5] = interstitial  ("1" or "0")
-- ARGV[6] = created_at    (unix nanoseconds, as a decimal string; also the ZSET score)
-- ARGV[7] = expires_at    (unix nanoseconds, "0" means never)
-- ARGV[8] = ttl_seconds   ("0" means no TTL is set on the link key)
--
-- Returns 1 if the record was created, 0 if the code already existed.
-- redisstore maps 0 to cairn.ErrCodeExists.
--
-- The EXISTS check and every write below run inside this one script, which
-- Redis executes as a single atomic unit: no other client can observe a
-- link record without its owner-index entry, or vice versa, and nothing here
-- is written at all if the code already exists (SR-18, NFR-11).

if redis.call('EXISTS', KEYS[1]) == 1 then
  return 0
end

redis.call('HSET', KEYS[1],
  'code', ARGV[1],
  'owner_id', ARGV[2],
  'dest_raw', ARGV[3],
  'vanity', ARGV[4],
  'interstitial', ARGV[5],
  'created_at', ARGV[6],
  'expires_at', ARGV[7],
  'revoked_at', '0')

if tonumber(ARGV[8]) > 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[8])
end

redis.call('ZADD', KEYS[2], ARGV[6], ARGV[1])

return 1
