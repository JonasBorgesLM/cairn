package redisstore

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/JonasBorgesLM/cairn"
)

// keySchema is the namespace and schema version every key carries, so this
// store can share a Redis instance without colliding with a host's own keys,
// and so the record format can migrate later (SR-22).
const keySchema = "cairn:v1"

// linkKey is the HASH holding one link record. code reaches this function
// only after the Shortener has validated it against the configured Alphabet
// (SR-21) — this package does not re-validate, and does not need to: Redis
// keys are binary-safe regardless of a code's content, so there is no
// delimiter to escape and no injection surface in the key text itself.
func linkKey(c cairn.Code) string {
	return keySchema + ":link:" + string(c)
}

// ownerKey is the ZSET backing OwnerLister for one owner, scored by
// CreatedAt. The unowned bucket (ownerID == "") is a key like any other.
func ownerKey(ownerID string) string {
	return keySchema + ":owner:" + ownerID
}

// hitsKey is the STRING counter backing Counter for one code.
func hitsKey(c cairn.Code) string {
	return keySchema + ":hits:" + string(c)
}

// destKey is the dedup index STRING, scoped per owner (ADR-0013). It keys on
// a hash of (ownerID, destination), never on the destination text itself:
// key names appear in SCAN output, MONITOR and slow-query logs, none of which
// are covered by Destination's redaction (SR-15).
func destKey(ownerID string, d cairn.Destination) string {
	sum := sha256.Sum256([]byte(ownerID + "\x00" + d.Raw()))
	return keySchema + ":dest:" + hex.EncodeToString(sum[:])
}
