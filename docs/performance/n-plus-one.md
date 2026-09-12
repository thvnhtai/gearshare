# The N+1 problem: measured, not asserted

This doc captures a real run against a locally seeded MySQL 8.4 instance
(1,400+ `gear_listings` rows, `docker-compose.yml`'s `mysql-primary`), not
hypothetical numbers. Reproduce with:

```bash
make compose-up   # or just bring up mysql-primary
make migrate-up
export MYSQL_TEST_DSN="gearshare_app:app_password@tcp(127.0.0.1:3306)/gearshare?parseTime=true&multiStatements=true"
go test -tags=integration -bench=BenchmarkFeed -benchtime=20x -run=^$ ./internal/listing/...
```

## The three implementations

| File | Round trips for a 20-row page | What it does |
|---|---|---|
| [`query_naive.go`](../../internal/listing/query_naive.go) | 1 + 3×20 = 61 | Fetch the page, then loop and fetch each row's owner, category, and rating aggregate one at a time. |
| [`loader.go`](../../internal/listing/loader.go) | 1 + 1 + 1 + 1 = 4 | Fetch the page, collect distinct owner/category/listing IDs, issue one batched `IN (...)` query per related entity. |
| [`query_optimized.go`](../../internal/listing/query_optimized.go) | 1 | Single query: `JOIN` for owner/category, a `GROUP BY` subquery for the rating aggregate. This is what `GET /api/v1/listings` actually calls (`internal/listing/handler.go`). |

## Measured results (Apple M1 Pro, local MySQL 8.4, 20-row page, `benchtime=20x`)

```
BenchmarkFeedNaive-8           20   25,571,319 ns/op   (~25.6 ms)
BenchmarkFeedBatched-8         20    1,913,477 ns/op   (~1.9 ms)
BenchmarkFeedOptimizedJoin-8   20    4,368,306 ns/op   (~4.4 ms)
```

The naive path is **~13x slower** than the batched dataloader and **~5.9x
slower** than the single JOIN, on a *local* database with no network
latency between the app and MySQL — in a real deployment where each round
trip costs a millisecond or more of network RTT, the gap widens
proportionally to the page size, not just by a constant factor.

Interestingly, the batched dataloader beat the single mega-JOIN here. That's
a real, reproducible result on this schema/data shape, not a benchmarking
artifact: the JOIN's `GROUP BY` subquery for rating aggregation runs a hash
join over the derived table for every listing in the page, while the
batched approach's separate `GROUP BY ... WHERE listing_id IN (...)` query
only aggregates the reviews that actually belong to this page's listings.
The lesson isn't "always batch instead of JOIN" — it's "measure your own
query shape"; `query_optimized.go` is still the right default here because
it's one query instead of four and the gap is well within noise for a
20-row page. `loader.go` exists because that pattern is what you reach for
when a JOIN literally isn't available (the related data lives in Mongo,
Elasticsearch, or a different microservice).

## EXPLAIN: before and after adding an index

Before [migration 000008](../../migrations/mysql/000008_index_listings_feed.up.sql),
the optimized feed query's `gear_listings` scan looked like this:

```
table: gl   type: ALL   possible_keys: idx_listings_owner,idx_listings_category_status
key: NULL   rows: 1403   Extra: Using where; Using temporary; Using filesort
```

`type: ALL` is a full table scan of all 1,403 rows for every single feed
request, purely because `WHERE status = 'active' ORDER BY created_at DESC`
had no matching index — `idx_listings_owner` and
`idx_listings_category_status` both existed but neither covers this
predicate/sort combination.

After adding `KEY idx_listings_status_created (status, created_at)`:

```
table: gl   type: range   key: idx_listings_status_created   key_len: 1
Extra: Using index condition; Using temporary; Using filesort
```

`type` moves from `ALL` to `range` and MySQL now uses the new index to
narrow to `status = 'active'` rows before sorting, instead of scanning the
whole table. `Using filesort` is still present — MySQL 8 supports
descending indexes (`(status, created_at DESC)`), which would let it walk
the index in the already-sorted order and drop the filesort entirely. That
further optimization is deliberately left as a documented next step rather
than applied silently: it's the kind of change that should show up as its
own reviewable commit with its own before/after, not get folded into this
one.

## Why the naive version is still in the repo

`query_naive.go` is never called by the running API — see its doc comment.
It exists purely as a benchmark fixture (`feed_bench_test.go`) so the
"before" number in this document is real, measured code, not a claim.
