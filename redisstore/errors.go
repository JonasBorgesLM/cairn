package redisstore

import (
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/JonasBorgesLM/cairn"
)

// mapErr translates a go-redis error into a cairn sentinel, so no go-redis
// type ever needs to appear in a caller's error handling (NFR-07). redis.Nil
// — a miss — becomes ErrCodeNotFound; anything else (a connection failure, a
// timeout, a command error) becomes ErrStoreUnavailable, fail-closed (SR-20):
// an unclassified failure is treated as the store being down, never as
// success and never as some other unexamined error shape a caller might
// mishandle.
//
// The cause is wrapped, not discarded, so errors.As still reaches it for a
// consumer who wants the detail — they just do not need go-redis imported to
// get the sentinel comparison right.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, redis.Nil) {
		return cairn.ErrCodeNotFound
	}
	return fmt.Errorf("%w: %w", cairn.ErrStoreUnavailable, err)
}
