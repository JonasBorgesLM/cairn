// Package redisstore implements cairn.Store on Redis.
//
// Redis is the single source of truth for a deployment using this store, and
// that carries an operational contract which is part of the library's contract
// rather than an assumption left to the operator:
//
//   - AOF with appendfsync everysec. RDB-only snapshots lose every link created
//     since the last save.
//   - maxmemory-policy noeviction. Any allkeys-* policy silently deletes links
//     under memory pressure, turning a memory spike into permanent link loss
//     with no error anywhere. This store implements cairn.EvictionChecker and
//     cairn.New refuses to start against a misconfigured instance.
//   - A replica, and a restore procedure that has actually been exercised.
//   - A dedicated database index. This package namespaces its keys; FLUSHDB
//     does not respect namespaces.
//
// See docs/adr/0008-store-contract-and-durability.md.
package redisstore
