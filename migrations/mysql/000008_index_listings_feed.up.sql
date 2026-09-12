-- Index-tuning example referenced in docs/performance/n-plus-one.md: without
-- this, the optimized feed query's EXPLAIN showed `type: ALL` on
-- gear_listings plus "Using temporary; Using filesort" for
-- `WHERE status = 'active' ORDER BY created_at DESC` — a full table scan
-- that gets worse as the catalog grows, independent of the N+1 fix.
ALTER TABLE gear_listings
    ADD KEY idx_listings_status_created (status, created_at);
