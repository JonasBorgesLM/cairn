package redisstore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/JonasBorgesLM/cairn"
)

// defaultExpiryGrace matches cairn's own default (ADR-0009). Store never
// receives the Shortener's configured grace directly — Save takes only a
// *cairn.Link — so this package holds its own, independently configurable
// copy. If a consumer changes cairn's WithExpiryGrace, changing this to
// match keeps the native TTL aligned with where cairn considers the grace
// window to end; a mismatch only shifts when Redis garbage-collects the
// record, never the correctness of cairn's own logical expiry check, which
// evaluates Link.ExpiresAt regardless of what TTL Redis was given.
const defaultExpiryGrace = 30 * 24 * time.Hour

// config accumulates Option values for New.
type config struct {
	password    string
	db          int
	expiryGrace time.Duration
}

// Option configures a Store at construction.
type Option func(*config)

// WithPassword sets the Redis AUTH password.
func WithPassword(p string) Option { return func(c *config) { c.password = p } }

// WithDB selects the Redis logical database index. ADR-0008 recommends a
// dedicated database index regardless, since this package's keys are
// namespaced (SR-22) but a shared FLUSHDB is not.
func WithDB(n int) Option { return func(c *config) { c.db = n } }

// WithExpiryGrace overrides the native-TTL grace window past a link's
// logical ExpiresAt (default 30 days, matching cairn's own default). See the
// defaultExpiryGrace doc comment for why this is configured here rather than
// derived from the Shortener that uses this Store.
func WithExpiryGrace(d time.Duration) Option { return func(c *config) { c.expiryGrace = d } }

// Store implements cairn.Store on Redis, along with cairn.OwnerLister,
// cairn.DestIndex, cairn.Counter and cairn.EvictionChecker. See the package
// doc comment for the operational contract Redis must satisfy.
type Store struct {
	client      *redis.Client
	expiryGrace time.Duration
}

// New connects to Redis at addr, which may be a plain "host:port" or a
// redis:// / rediss:// URL, and returns a ready Store. New does not itself
// verify connectivity; cairn.New does that indirectly by calling
// EvictionCheck when a Store implements EvictionChecker, which this one
// does — an unreachable Redis at startup fails that check closed rather than
// silently passing (ADR-0008).
func New(addr string, opts ...Option) (*Store, error) {
	cfg := config{expiryGrace: defaultExpiryGrace}
	for _, opt := range opts {
		opt(&cfg)
	}

	var redisOpts *redis.Options
	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		parsed, err := redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("redisstore: parsing address: %w", err)
		}
		redisOpts = parsed
	} else {
		redisOpts = &redis.Options{Addr: addr}
	}
	if cfg.password != "" {
		redisOpts.Password = cfg.password
	}
	if cfg.db != 0 {
		redisOpts.DB = cfg.db
	}

	return &Store{client: redis.NewClient(redisOpts), expiryGrace: cfg.expiryGrace}, nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	return s.client.Close()
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func unixNanoOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// ttlSeconds computes the native TTL for l: time remaining until its logical
// expiry plus the configured grace window (ADR-0009). Zero means no TTL is
// set — either l never expires, or it is already past its grace window,
// which EXPIRE with a non-positive value would delete immediately rather
// than leave for cairn's own logical check to report as ErrLinkExpired.
func (s *Store) ttlSeconds(l *cairn.Link) int64 {
	if l.ExpiresAt.IsZero() {
		return 0
	}
	remaining := time.Until(l.ExpiresAt.Add(s.expiryGrace))
	if remaining <= 0 {
		return 0
	}
	seconds := int64(remaining.Seconds())
	if seconds == 0 {
		seconds = 1 // round up: a sub-second remainder must not mean "no TTL".
	}
	return seconds
}

// Save implements cairn.Store. It is conditional: an existing code returns
// cairn.ErrCodeExists and the record is never overwritten (SR-18).
func (s *Store) Save(ctx context.Context, l *cairn.Link) error {
	res, err := saveScript.Run(ctx, s.client,
		[]string{linkKey(l.Code), ownerKey(l.OwnerID)},
		string(l.Code),
		l.OwnerID,
		l.Dest.Raw(),
		boolFlag(l.Vanity),
		boolFlag(l.Interstitial),
		strconv.FormatInt(l.CreatedAt.UnixNano(), 10),
		strconv.FormatInt(unixNanoOrZero(l.ExpiresAt), 10),
		strconv.FormatInt(s.ttlSeconds(l), 10),
	).Result()
	if err != nil {
		return mapErr(err)
	}
	n, ok := res.(int64)
	if !ok {
		return fmt.Errorf("redisstore: save.lua returned %T, want int64", res)
	}
	if n == 0 {
		return cairn.ErrCodeExists
	}
	return nil
}

// Load implements cairn.Store.
func (s *Store) Load(ctx context.Context, c cairn.Code) (*cairn.Link, error) {
	vals, err := s.client.HGetAll(ctx, linkKey(c)).Result()
	if err != nil {
		return nil, mapErr(err)
	}
	if len(vals) == 0 {
		return nil, cairn.ErrCodeNotFound
	}
	return decodeLink(c, vals)
}

func decodeLink(c cairn.Code, vals map[string]string) (*cairn.Link, error) {
	var dest cairn.Destination
	if raw := vals["dest_raw"]; raw != "" {
		d, err := cairn.ParseDestination(raw)
		if err != nil {
			return nil, fmt.Errorf("redisstore: stored destination for %q failed to parse: %w", c, err)
		}
		dest = d
	}

	createdAt, err := decodeUnixNano(vals["created_at"])
	if err != nil {
		return nil, fmt.Errorf("redisstore: decoding created_at for %q: %w", c, err)
	}
	expiresAt, err := decodeUnixNano(vals["expires_at"])
	if err != nil {
		return nil, fmt.Errorf("redisstore: decoding expires_at for %q: %w", c, err)
	}
	revokedAt, err := decodeUnixNano(vals["revoked_at"])
	if err != nil {
		return nil, fmt.Errorf("redisstore: decoding revoked_at for %q: %w", c, err)
	}

	return &cairn.Link{
		Code:         c,
		Dest:         dest,
		OwnerID:      vals["owner_id"],
		Vanity:       vals["vanity"] == "1",
		Interstitial: vals["interstitial"] == "1",
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
		RevokedAt:    revokedAt,
	}, nil
}

// decodeUnixNano parses a stored timestamp field. "0" and "" both mean the
// zero time.Time, matching Link's own "zero means never/not revoked"
// convention.
func decodeUnixNano(s string) (time.Time, error) {
	if s == "" || s == "0" {
		return time.Time{}, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, n).UTC(), nil
}

// Revoke implements cairn.Store.
func (s *Store) Revoke(ctx context.Context, c cairn.Code, at time.Time, purgeDestination bool) error {
	res, err := revokeScript.Run(ctx, s.client,
		[]string{linkKey(c)},
		strconv.FormatInt(at.UnixNano(), 10),
		boolFlag(purgeDestination),
	).Result()
	if err != nil {
		return mapErr(err)
	}
	n, ok := res.(int64)
	if !ok {
		return fmt.Errorf("redisstore: revoke.lua returned %T, want int64", res)
	}
	if n == 0 {
		return cairn.ErrCodeNotFound
	}
	return nil
}

// ListByOwner implements cairn.OwnerLister, backed by a ZSET scored by
// CreatedAt (ADR-0010).
func (s *Store) ListByOwner(ctx context.Context, ownerID string, after cairn.Cursor, limit int) ([]*cairn.Link, cairn.Cursor, error) {
	start := 0
	if after != "" {
		n, err := strconv.Atoi(string(after))
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("redisstore: invalid cursor %q", after)
		}
		start = n
	}

	codes, err := s.client.ZRange(ctx, ownerKey(ownerID), int64(start), int64(start+limit-1)).Result()
	if err != nil {
		return nil, "", mapErr(err)
	}

	links := make([]*cairn.Link, 0, len(codes))
	for _, code := range codes {
		link, err := s.Load(ctx, cairn.Code(code))
		if err != nil {
			if errors.Is(err, cairn.ErrCodeNotFound) {
				continue // a collected record's index entry is stale, not an error.
			}
			return nil, "", err
		}
		links = append(links, link)
	}

	var next cairn.Cursor
	if len(codes) == limit {
		total, err := s.client.ZCard(ctx, ownerKey(ownerID)).Result()
		if err == nil && int64(start+limit) < total {
			next = cairn.Cursor(strconv.Itoa(start + limit))
		}
	}
	return links, next, nil
}

// LookupByDest implements cairn.DestIndex, scoped per owner (ADR-0013).
func (s *Store) LookupByDest(ctx context.Context, ownerID string, d cairn.Destination) (cairn.Code, error) {
	val, err := s.client.Get(ctx, destKey(ownerID, d)).Result()
	if err != nil {
		return "", mapErr(err)
	}
	return cairn.Code(val), nil
}

// IndexDest implements cairn.DestIndex, scoped per owner (ADR-0013).
func (s *Store) IndexDest(ctx context.Context, ownerID string, d cairn.Destination, c cairn.Code) error {
	return mapErr(s.client.Set(ctx, destKey(ownerID, d), string(c), 0).Err())
}

// Count implements cairn.Counter.
func (s *Store) Count(ctx context.Context, c cairn.Code) error {
	return mapErr(s.client.Incr(ctx, hitsKey(c)).Err())
}

// CountOf returns the number of times Count has recorded c. It exists for a
// consumer's own tests, the same reason memstore's equivalent does.
func (s *Store) CountOf(ctx context.Context, c cairn.Code) (int64, error) {
	n, err := s.client.Get(ctx, hitsKey(c)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

// EvictionCheck implements cairn.EvictionChecker. cairn.New calls it and
// refuses to start against anything other than maxmemory-policy noeviction:
// any allkeys-* policy silently deletes links under memory pressure, turning
// a memory spike into permanent link loss with no error anywhere (ADR-0008,
// T-12).
func (s *Store) EvictionCheck(ctx context.Context) error {
	vals, err := s.client.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		return mapErr(err)
	}
	policy, ok := vals["maxmemory-policy"]
	if !ok {
		return fmt.Errorf("redisstore: CONFIG GET maxmemory-policy returned no value")
	}
	if policy != "noeviction" {
		return fmt.Errorf(
			"redisstore: maxmemory-policy is %q, want \"noeviction\" (ADR-0008): "+
				"any allkeys-* policy silently deletes links under memory pressure; "+
				"run `redis-cli CONFIG SET maxmemory-policy noeviction`", policy)
	}
	return nil
}
