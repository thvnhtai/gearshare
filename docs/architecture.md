# GearShare architecture

GearShare is a peer-to-peer outdoor gear rental marketplace, built as a
reference architecture: every major backend/infra pattern in the project's
requirement checklist is implemented with real, running code on at least
the core listing-browse and booking-creation paths. This document is the
map; see the ADRs in `docs/adr/` for the reasoning behind specific
decisions.

## Architectural patterns, concretely

| Pattern | Where |
|---|---|
| Modular monolith | `cmd/api` + most of `internal/` — see [ADR 0003](adr/0003-service-boundaries.md). |
| Microservices / SOA | `cmd/notification-service`, `cmd/search-indexer` — own process, own Dockerfile, contract-only communication (gRPC + Kafka/RabbitMQ), zero shared database. |
| Serverless | `cmd/thumbnail-fn` — a real `aws-lambda-go` handler (`main_lambda.go`), also runnable locally as a RabbitMQ-consumer process (`main_worker.go`) sharing the same `Handler()`. |
| Service mesh | Illustrative only: `deployments/k8s/mesh/*.yaml` (Istio `VirtualService`/`DestinationRule` shape) — not a deployed control plane. See the plan's "thin-but-real" list. |
| Twelve-Factor | [`docs/twelve-factor.md`](twelve-factor.md), factor-by-factor against actual files. |

## Request flow: creating a booking

```
Browser (booking.html)
  → POST /api/v1/bookings  [JWT auth]
  → internal/booking/handler.go
  → internal/booking/service.go
  → internal/booking/tx.go (ONE sql.Tx: overlap check + booking insert + availability_block insert)
  → internal/booking/service.go: cache.InvalidateDetail(listingID), publishEvent(BookingCreated)
  → internal/booking/handler.go: hub.PublishOwner(ownerID, ...)  → SSE to dashboard.html
  → Kafka topic booking.events → search-indexer (Elasticsearch upsert), audit consumer
  → RabbitMQ notifications exchange → notification-service → email
```

Every arrow above is real, wired code, not a diagram of intent — see
`internal/booking/` for the synchronous path and `internal/eventbus`/
`internal/queue` for the asynchronous fan-out.

## Auth styles, mapped to real endpoints

| Style | Endpoint(s) | Why this style here |
|---|---|---|
| JWT | `POST /api/v1/auth/{register,login}`, all of `/api/v1/*` protected routes | Primary API auth for the frontend — stateless, works across `cmd/api` replicas. |
| OAuth 2.0 | `GET /api/v1/auth/oauth/google(/callback)` | "Log in with Google" for renters who don't want a password. |
| OpenID Connect | Same Google flow, `internal/auth/oidc.go`'s `id_token` verification — layered on top of, and coded separately from, the OAuth2 access-token exchange, because OIDC (identity) and OAuth2 (authorization) answer different questions even when they share one button. |
| Basic Auth | `/internal/admin/health-detailed`, `/internal/debug/pprof/*` | Operator-only, low-traffic, never exposed past Nginx to the public internet — a different, simpler credential store than end-user auth. |
| API Key / Token | `/partner/v1/*` | Machine-to-machine partner integrations have no login flow and no session; a long-lived, revocable, SHA-256-hashed key fits that shape better than a JWT. |
| Cookie + CSRF | `/admin/dashboard/*` (server-rendered `html/template`) | A classic server-rendered admin surface, where a cookie session plus CSRF token is the standard, well-understood defense — no JSON API client involved. |
| SAML | `/business/admin/login`, `POST /saml/acs` | Enterprise B2B customers who mandate SSO through their own IdP — a distinct protocol from consumer OAuth, aimed at a distinct (fictitious) B2B admin portal. |

## REST endpoint inventory (representative)

```
POST /api/v1/auth/register | login | refresh          JWT
GET  /api/v1/auth/oauth/google (+ /callback)           OAuth2 + OIDC
GET  /api/v1/listings, /api/v1/listings/{id}           Redis cache-aside + ETag
POST /api/v1/listings                                  JWT (owner)
POST /api/v1/bookings                                  JWT — core transactional path
POST /api/v1/bookings/{id}/approve|reject|cancel|complete
GET  /api/v1/bookings                                  JWT — owner's bookings (dashboard initial load)
GET  /api/v1/bookings/events                           SSE (owner dashboard live feed)
GET  /api/v1/bookings/{id}/poll                         long-poll fallback
GET  /ws/listings/{id}                                  WebSocket — live availability
POST /api/v1/reviews
GET  /partner/v1/listings, POST /partner/v1/webhooks/insurance   API-Key auth
GET  /internal/admin/health-detailed, /internal/debug/pprof/*    Basic Auth
GET/POST /admin/dashboard/*                             Cookie session + CSRF
GET /saml/metadata, POST /saml/acs, GET /business/admin/login    SAML
```

## gRPC services (`gearshare.v1`, `api/proto/gearshare/v1/`)

- `UserInternalService.GetUserContact` — implemented by the monolith,
  called by `notification-service` so notification payloads never need to
  carry PII themselves.
- `SearchInternalService.Search` / `IndexListing` — implemented by
  `search-indexer`, called by the monolith's listing-feed path with a
  circuit breaker (`internal/middleware/breaker.go`) and a MySQL `LIKE`
  fallback (`internal/search/fallback_mysql.go`).
- `NotificationService.SendTransactional` — implemented by
  `notification-service`, for the synchronous/urgent notification path
  (complementing the async RabbitMQ queue for bulk email).

## Kafka topics / RabbitMQ queues

- **Kafka** `booking.events` (key: `booking_id`) — `BookingCreated/Approved/
  Rejected/Cancelled/Completed/Disputed`. Consumers: `search-indexer`
  (Elasticsearch upsert), an audit-log consumer.
- **Kafka** `listing.events` — `ListingCreated/Updated/Deleted`. Consumer:
  `search-indexer`.
- **RabbitMQ** exchange `notifications` → queue `notifications.email`
  (+ DLX `notifications.dlx` → `notifications.email.dlq`). Consumer:
  `notification-service`.
- **RabbitMQ** exchange `media` → queue `media.thumbnail` (+ DLX pattern).
  Consumer: `thumbnail-fn`.

## Data store boundaries

- **MySQL**: everything transactional and relational — users, listings,
  availability, bookings, reviews, api_keys, refresh_tokens,
  oauth_identities. See [ERD](erd/gearshare-erd.md).
- **MongoDB**: `listing_specs` (flexible per-category attributes — a tent's
  spec sheet and a camera's spec sheet share nothing), `damage_reports`
  (freeform incident write-ups with photo arrays, created from a booking
  dispute).
- **Redis**: cache-aside for the listing feed/detail
  (`internal/cache/listing_cache.go`), invalidated explicitly on booking
  state changes; also the backing store for the distributed rate limiter
  once the API runs as more than one replica.
- **Elasticsearch**: the `gear_listings` search index, fed asynchronously
  from Kafka by `search-indexer` — never written to directly by the
  monolith.

## Resilience patterns

| Pattern | Where |
|---|---|
| Graceful degradation | `internal/search/fallback_mysql.go` — ES/gRPC unavailable → MySQL `LIKE` query. |
| Throttling | `internal/middleware/ratelimit.go` (per-IP, in-process) → Redis-backed for multi-replica. |
| Back pressure | Bounded channels between broker fetch and worker pool in `internal/eventbus`/`internal/queue`; RabbitMQ consumer QoS `prefetch`. |
| Load shifting | Thumbnail generation and bulk email are queued (RabbitMQ), never done inline in a request handler. |
| Circuit breaker | `internal/middleware/breaker.go` (`sony/gobreaker`) wrapping the gRPC search client and the indexer's Elasticsearch client. |
