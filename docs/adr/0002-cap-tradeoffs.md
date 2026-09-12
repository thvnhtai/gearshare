# ADR 0002: CAP tradeoffs and transaction failure modes

## Status
Accepted.

## CAP positioning of each store

| Store | Role | CAP leaning | Why |
|---|---|---|---|
| MySQL (primary) | Bookings, listings, users — the system of record | **CP** | A booking's atomicity (see below) is the one property GearShare cannot compromise: two renters must never both win the same date range. Under a network partition, GearShare would rather reject a booking write than risk a lost-update. |
| MySQL (replica) | Read-only listing/search fallback | CP, with bounded staleness | Async binlog replication means a replica read can lag the primary by a small, monitored window — acceptable for browsing, never used for the booking-creation path itself (`internal/db/mysql.go`'s `Reader()` is only called from read paths). |
| Elasticsearch | Search index | **AP** | Fed asynchronously from Kafka (`internal/eventbus`); a search result can be briefly stale after a listing is created/edited. `internal/search/fallback_mysql.go` exists precisely because ES availability, not consistency, is what's being traded for here — GearShare would rather serve a stale-but-present search result (or fall back to MySQL `LIKE`) than a 500. |
| Kafka | Durable event stream (booking/listing lifecycle) | **AP**-leaning | At-least-once delivery, consumer-driven ordering per partition key. A duplicate `BookingCreated` event is idempotent-safe for its consumers (upsert semantics in search-indexer); a dropped event would only delay search-index freshness, never corrupt the booking record itself (MySQL remains the source of truth). |
| MongoDB | `listing_specs`, `damage_reports` | AP-leaning (default read concern) | Neither collection participates in the booking transaction — a damage report can be written a few seconds after a dispute is filed without any correctness impact on the booking state machine. |

## Failure modes handled explicitly

- **Deadlock / lock-wait-timeout on booking creation**: MySQL errors 1213 and
  1205 are retried with jittered backoff up to 3 times
  (`internal/db/tx.go`'s `WithinTransaction`) before surfacing to the
  caller. This is the only retry policy in the codebase that targets a
  specific, named failure mode rather than retrying blindly.
- **Partial write**: the booking row and its `availability_blocks` row are
  written in one `sql.Tx` (`internal/booking/tx.go`) — a failure after the
  booking insert but before the availability-block insert rolls back both,
  so there is no code path that can produce a booking without a
  corresponding availability block (or vice versa).
- **Broker unavailable at event-publish time**: `internal/booking/events.go`
  treats a publish failure as non-fatal to the booking itself (logged, not
  returned as an error) — the booking is already durably committed in
  MySQL before the event is published. This is a deliberate AP choice for
  the *side effects* of a booking (search indexing, notification) that does
  not compromise the CP guarantee for the booking record itself.

## Consequences
Two different consistency models coexist by design, not by accident: the
booking write path is strict (CP, synchronous, transactional) while
everything downstream of "a booking happened" (search, notifications,
audit log) is eventually consistent (AP, asynchronous, event-driven). A
reviewer should expect a freshly created listing to occasionally be
missing from search results for a few hundred milliseconds, and should
never expect two renters to successfully book the same gear for
overlapping dates.
