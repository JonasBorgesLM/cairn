package redisstore

import (
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/JonasBorgesLM/cairn"
)

func TestMapErr_Nil(t *testing.T) {
	if got := mapErr(nil); got != nil {
		t.Fatalf("mapErr(nil) = %v, want nil", got)
	}
}

func TestMapErr_RedisNilBecomesErrCodeNotFound(t *testing.T) {
	got := mapErr(redis.Nil)
	if !errors.Is(got, cairn.ErrCodeNotFound) {
		t.Fatalf("mapErr(redis.Nil) = %v, want errors.Is(_, ErrCodeNotFound)", got)
	}
	if errors.Is(got, cairn.ErrStoreUnavailable) {
		t.Fatalf("mapErr(redis.Nil) = %v, must not also be ErrStoreUnavailable", got)
	}
}

// Connection and timeout failures become ErrStoreUnavailable, with the cause
// still reachable through errors.As/errors.Is for a consumer who wants it --
// the mapping wraps rather than discards.
func TestMapErr_OtherErrorsBecomeErrStoreUnavailable(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	got := mapErr(cause)
	if !errors.Is(got, cairn.ErrStoreUnavailable) {
		t.Fatalf("mapErr(%v) = %v, want errors.Is(_, ErrStoreUnavailable)", cause, got)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("mapErr(%v) = %v, want the cause still reachable via errors.Is", cause, got)
	}
}
