package redisstore

import (
	_ "embed"

	"github.com/redis/go-redis/v9"
)

// The record and its owner-index entry are written by save.lua, and a
// revocation by revoke.lua, so that neither pair can diverge under a partial
// failure (NFR-11, ADR-0010). Lua scripts live in their own files, embedded,
// never as Go string literals — a .lua file is readable and lintable as Lua
// on its own, which a quoted Go string is not.

//go:embed save.lua
var saveScriptSource string

//go:embed revoke.lua
var revokeScriptSource string

// saveScript and revokeScript wrap the embedded sources as go-redis Scripts,
// which cache the SHA server-side after the first EVAL and use EVALSHA
// afterward — a detail of this package's talking to Redis, not of the
// contract it exposes.
var (
	saveScript   = redis.NewScript(saveScriptSource)
	revokeScript = redis.NewScript(revokeScriptSource)
)
