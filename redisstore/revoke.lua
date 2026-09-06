-- revoke.lua atomically tombstones a link record, optionally clearing its
-- destination in the same write.
--
-- KEYS[1] = link key (HASH)
-- ARGV[1] = revoked_at (unix nanoseconds, as a decimal string)
-- ARGV[2] = purge ("1" or "0")
--
-- Returns 1 if the record existed and was revoked, 0 if the code was not
-- found. redisstore maps 0 to cairn.ErrCodeNotFound.
--
-- The existence check and the write happen in this one atomic script so that
-- revoking a code that was never issued can never create a phantom record
-- for it (a plain HSET on a missing key would do exactly that).

if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end

redis.call('HSET', KEYS[1], 'revoked_at', ARGV[1])

if ARGV[2] == '1' then
  redis.call('HSET', KEYS[1], 'dest_raw', '')
end

return 1
