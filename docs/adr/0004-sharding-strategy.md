# ADR 0004: Sharding strategy

## Status
Accepted (logical sharding only — see "what's not built" below).

## Decision
Bookings and availability data are logically partitioned by
`hash(owner_id) % N` — implemented, unit-tested, real code at
[`internal/availability/sharding.go`](../../internal/availability/sharding.go).
All of a given owner's listings/bookings/availability route to the same
shard, so an owner's dashboard query and a booking-creation transaction for
one of their listings never need a cross-shard join or a distributed
transaction.

`hash(owner_id)`, not `hash(listing_id)` or `hash(booking_id)`, because the
hottest cross-row query in the system is "everything this owner owns" (the
SSE dashboard feed, `internal/booking/repository.go`'s `ListForOwner`) —
sharding by owner keeps that query single-shard regardless of how many
listings or bookings that owner accumulates.

## What's actually built vs. what's documented
`ShardRouter.ShardFor(ownerID)` today maps every owner to a shard index in
`[0, N)`, and every one of those indices happens to point at the same
physical MySQL instance (`docker-compose.yml` runs one `mysql-primary`).
This is a deliberate, disclosed simplification for a portfolio project — see
the plan's "explicitly thin-but-real treatments." What makes it a real
demonstration of the pattern rather than a documentation-only claim:

- The hash function is deterministic and unit-tested for range and
  distribution (`internal/availability/sharding_test.go`).
- The router is a real interface boundary (`ShardRouter`), not a comment —
  swapping `internal/db.DB` for a `map[int]*db.DB` keyed by shard index and
  changing `ShardRouter.ShardFor` callers to look up the right connection
  pool is the *only* change needed to go from logical to physical sharding.
  No query code, no repository code, and no application logic above the
  connection-selection layer would need to change.

## Why not build N real physical shards
Standing up N MySQL clusters with real cross-shard query restrictions would
demonstrate operational complexity, not the sharding *decision* itself —
and would make every other part of this project (migrations, replication,
transactions) need to be re-explained per-shard. The routing logic is the
part of "sharding" that's actually a design decision; running N databases
is infrastructure repetition of a decision already made once.

## Consequences
- If this system needed to shard for real, the migration path is: pick N,
  provision N MySQL instances, change `db.Connect` call sites that
  currently take one `config.MySQLConfig` to take N, and route through
  `ShardRouter.ShardFor(ownerID)` at the connection-selection layer.
- Cross-shard queries (e.g., a platform-wide admin report across all
  owners) would need a fan-out/scatter-gather layer that does not exist
  today and is out of scope for this project.
