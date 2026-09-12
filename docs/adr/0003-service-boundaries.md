# ADR 0003: Where the monolith ends and services begin

## Status
Accepted.

## Decision
GearShare is a **modular monolith** (`cmd/api`) for everything that reads
or writes MySQL directly — users, listings, availability, bookings,
reviews. Two capabilities are extracted as separate services with their own
`cmd/` binary, own Dockerfile, and zero direct MySQL access:

- **`cmd/notification-service`** — owns email/SMS delivery. Talks to the
  monolith only via `UserInternalService.GetUserContact` (gRPC) to resolve
  contact details, and consumes RabbitMQ's `notifications.email` queue for
  work items. It never sees a MySQL connection string.
- **`cmd/search-indexer`** — owns the Elasticsearch index. Consumes
  `booking.events`/`listing.events` from Kafka and exposes
  `SearchInternalService.Search`/`IndexListing` over gRPC to the monolith.
  It never sees a MySQL connection string either — its only knowledge of
  "what a listing looks like" comes from the event payloads it consumes.

`cmd/thumbnail-fn` is a third, differently-shaped extraction: a
**serverless function** (see ADR-adjacent notes in `docs/architecture.md`),
not a long-running service — it has no persistent state and no listener
beyond whatever invokes it (Lambda event or a RabbitMQ consumer loop
sharing the same `Handler()`).

## Why these two, and no more
Both extracted services share three properties that make them good
extraction candidates and everything left in the monolith does not:

1. **I/O-bound, not data-owning.** Neither needs a row-level transaction
   guarantee — email delivery and search indexing are both naturally
   eventually-consistent operations (see ADR 0002).
2. **Independently scalable load shape.** A traffic spike on
   `GET /api/v1/listings` has nothing to do with notification-service's
   load, which spikes on booking *events*, not booking *reads*. Coupling
   their deployment to the monolith would mean over-provisioning one to
   satisfy the other.
3. **A real SOA/microservices boundary, not a network-attached function
   call.** Both communicate through a stable contract (a `.proto` service
   definition or a Kafka topic schema) that could be reimplemented in a
   different language without the monolith noticing.

Everything else — listings, bookings, reviews — stays in the monolith
because splitting them would only add network hops around a single MySQL
instance's transaction boundary, without any of the three properties above.
`internal/booking/tx.go`'s atomic booking+availability write is the clearest
example: it needs to be one `sql.Tx`, which means it needs to be one
process.

## Consequences
- The monolith is still the single largest blast radius in the system: if
  it's down, browsing and booking are down, full stop. That's accepted as
  correct for this project's scale — the two services that *are* split out
  are split out because their failure is tolerable (search goes stale,
  notifications queue up) in a way the booking path's failure is not.
- Adding a third extracted service later should be held to the same three-
  property test above, not to "microservices are good practice."
