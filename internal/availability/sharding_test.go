package availability

import "testing"

func TestShardRouter_Deterministic(t *testing.T) {
	router := NewShardRouter(4)
	first := router.ShardFor(42)
	for i := 0; i < 100; i++ {
		if got := router.ShardFor(42); got != first {
			t.Fatalf("ShardFor(42) not deterministic: got %d, want %d", got, first)
		}
	}
}

func TestShardRouter_WithinRange(t *testing.T) {
	router := NewShardRouter(8)
	for ownerID := int64(0); ownerID < 1000; ownerID++ {
		shard := router.ShardFor(ownerID)
		if shard < 0 || shard >= router.ShardCount() {
			t.Fatalf("ShardFor(%d) = %d, out of range [0,%d)", ownerID, shard, router.ShardCount())
		}
	}
}

func TestShardRouter_DistributesAcrossShards(t *testing.T) {
	router := NewShardRouter(4)
	counts := make(map[int]int)
	for ownerID := int64(0); ownerID < 10_000; ownerID++ {
		counts[router.ShardFor(ownerID)]++
	}
	if len(counts) != router.ShardCount() {
		t.Fatalf("expected owners to land on all %d shards, only hit %d", router.ShardCount(), len(counts))
	}
	for shard, count := range counts {
		if count < 1500 { // rough balance check, not a strict uniformity requirement
			t.Errorf("shard %d only received %d of 10000 owners, distribution looks skewed", shard, count)
		}
	}
}
