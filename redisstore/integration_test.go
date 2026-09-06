//go:build integration

// Package redisstore_test's integration suite runs against a real Redis via
// testcontainers, never miniredis (NFR-10): a fake that implements SetNX
// correctly proves nothing about the server that has to. It is gated behind
// the "integration" build tag so a plain `go test ./...` — the one CI and a
// consumer both run without Docker — never needs a container, and the CI
// job that does run it is explicit about skipping rather than silently
// passing when the tag or Docker is absent.
package redisstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/redisstore"
	"github.com/JonasBorgesLM/cairn/storetest"
)

// redisImage is pinned (issue #38): a floating tag makes today's green run
// say nothing about tomorrow's.
const redisImage = "redis:7.4-alpine"

// testRedis starts one Redis container for the calling test and returns its
// address plus a raw client for setup/assertions the cairn.Store contract
// does not expose (FLUSHALL between subtests, CONFIG SET for the eviction
// test, reading a key directly to check byte-identical). Using go-redis
// directly here is fine: this file is a test, not part of the exported API
// surface NFR-07 is about.
func testRedis(t *testing.T) (addr string, raw *redis.Client) {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		t.Fatalf("starting redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminating redis container: %v", err)
		}
	})

	addr, err = container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("getting redis endpoint: %v", err)
	}

	raw = redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = raw.Close() })

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := raw.Ping(waitCtx).Err(); err != nil {
		t.Fatalf("redis did not become ready: %v", err)
	}

	return addr, raw
}

func newStore(t *testing.T, addr string) *redisstore.Store {
	t.Helper()
	s, err := redisstore.New(addr)
	if err != nil {
		t.Fatalf("redisstore.New error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestConformance runs the shared cairn.Store conformance suite (SR-18)
// against a real Redis, flushing between subtests so each sees an empty
// keyspace as storetest.Run requires.
func TestConformance(t *testing.T) {
	addr, raw := testRedis(t)

	storetest.Run(t, func() cairn.Store {
		if err := raw.FlushAll(context.Background()).Err(); err != nil {
			t.Fatalf("FLUSHALL error = %v", err)
		}
		return newStore(t, addr)
	})
}
