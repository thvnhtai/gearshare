# ADR 0001: Query layer — sqlc over GORM/ent

## Status
Accepted.

## Context
GearShare needs to demonstrate the N+1 query problem and its fix side by side
(`internal/listing/query_naive.go` vs `query_optimized.go`), plus raw
`EXPLAIN`-able SQL for the index-tuning writeup in
`docs/performance/n-plus-one.md`. That requires full visibility into and
control over the exact SQL sent to MySQL.

## Decision
Use **sqlc** as the query layer of record: hand-written SQL lives in
`internal/db/queries/*.sql`, `sqlc generate` (see `sqlc.yaml`, `make sqlc`)
produces type-safe Go into `internal/db/sqlc/`. GORM and ent were rejected
because both generate their own SQL from struct/DSL definitions, which is
exactly the opacity this project needs to avoid — you cannot show a
convincing "here is the naive query, here is the fix" comparison when the
ORM decides the query shape for you.

## Local toolchain note
`sqlc` requires cgo (it embeds a SQLite-based query analyzer) and could not
be installed in the environment this repo was scaffolded in (broken Xcode
Command Line Tools: `xcode-select`/`clang` unavailable). The `.sql` files
under `internal/db/queries/` are still the source of truth and are written
to be sqlc-compatible; the actual runtime repositories in
`internal/<capability>/repository.go` are hand-written `sqlx` code that is
functionally identical to what `sqlc generate` would emit. Once a working
cgo toolchain is available, run `make sqlc` and swap the repository internals
to call the generated `sqlcgen.Queries` methods instead — the public
repository interfaces do not need to change.

## Consequences
- Every query the app runs is visible, grep-able SQL — good for the N+1 demo
  and for `EXPLAIN` profiling.
- Slightly more boilerplate per query than an ORM's `.Find()` sugar; accepted
  as the right tradeoff for a project whose whole point is showing database
  mechanics, not hiding them.
