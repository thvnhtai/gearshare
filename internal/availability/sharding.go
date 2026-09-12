package availability

import "hash/fnv"

// ShardRouter demonstrates GearShare's chosen sharding strategy: bookings
// and availability data are logically partitioned by hash(owner_id) % N,
// so all of a given owner's listings/bookings/availability land on the same
// shard — read/write locality for the operations that matter (an owner's
// dashboard, a booking transaction) without a cross-shard join.
//
// Today every shard index maps to the same physical MySQL instance
// (docs/adr/0004-sharding-strategy.md explains why a portfolio project
// doesn't stand up N real MySQL clusters) — but the routing function itself
// is real, deterministic, and unit-tested (sharding_test.go), so swapping
// ShardConnections to point at N distinct *db.DB instances is the only
// change needed to go from "logical" to "physical" sharding.
type ShardRouter struct {
	shardCount uint32
}

func NewShardRouter(shardCount int) *ShardRouter {
	if shardCount < 1 {
		shardCount = 1
	}
	return &ShardRouter{shardCount: uint32(shardCount)}
}

// ShardFor returns the shard index (0..shardCount-1) an owner's data lives
// on. FNV-1a is used purely for its speed and good avalanche behavior on
// small integer keys — no cryptographic property is needed since this is a
// routing decision, not a security boundary.
func (r *ShardRouter) ShardFor(ownerID int64) int {
	h := fnv.New32a()
	// Writing a fixed-width big-endian representation keeps the hash
	// deterministic across platforms/architectures.
	buf := [8]byte{}
	for i := 0; i < 8; i++ {
		buf[7-i] = byte(ownerID >> (8 * i))
	}
	_, _ = h.Write(buf[:])
	return int(h.Sum32() % r.shardCount)
}

func (r *ShardRouter) ShardCount() int {
	return int(r.shardCount)
}
