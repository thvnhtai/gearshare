# GearShare

[![CI](https://github.com/thvnhtai/gearshare/actions/workflows/ci.yml/badge.svg)](https://github.com/thvnhtai/gearshare/actions/workflows/ci.yml)

A peer-to-peer outdoor gear rental marketplace — and a reference
architecture. Every major backend/infrastructure pattern below is real,
wired code exercised against real MySQL, Redis, Kafka, RabbitMQ,
Elasticsearch, MongoDB, and (via `kind`) a real Kubernetes cluster, not
prose describing an intention. Where something is genuinely thinner than
the rest (SAML, service mesh, sharding), that's called out explicitly
rather than quietly implied — see [Explicitly thin-but-real
treatments](#explicitly-thin-but-real-treatments) below.

Owners list gear (tents, kayaks, cameras); renters browse, book date
ranges, owners approve/reject, bookings move through a lifecycle, renters
leave reviews. See [`docs/architecture.md`](docs/architecture.md) for the
full picture.

## Quickstart

```bash
git clone <this-repo> && cd gearshare
cp .env.example .env

# Bring up MySQL primary/replica + Redis (everything else is optional —
# see "Running the full stack" below for the complete picture).
docker compose up -d mysql-primary mysql-replica redis

# Apply migrations (needs a MySQL client; or use golang-migrate — see Makefile).
for f in migrations/mysql/*.up.sql; do
  mysql -h127.0.0.1 -P3306 -uroot -proot_password gearshare < "$f"
done

go run ./cmd/api
```

Then open [`web/index.html`](web/index.html) directly in a browser (or
serve it with any static file server), or run `scripts/seed.sh` to
populate some demo listings first.

## Running the full stack

```bash
bash scripts/gen-dev-certs.sh   # self-signed TLS cert for Nginx, once
docker compose up -d
```

This brings up: MySQL primary + replica (real binlog replication — run
`scripts/mysql-replication-setup.sh` once to wire it up), Redis, MongoDB,
Kafka + Zookeeper, RabbitMQ, Elasticsearch, Jaeger, Prometheus, Grafana,
the `api` monolith, both extracted microservices
(`notification-service`, `search-indexer`), the `thumbnail-fn` worker, and
Nginx in front of everything on `https://localhost` (self-signed —
your browser will warn, that's expected).

## What's here

| Requirement | Where | |
|---|---|---|
| Frontend: plain HTML/CSS/JS | [`web/`](web/) | no framework |
| Backend: Go | [`cmd/`](cmd/), [`internal/`](internal/) | |
| MySQL + migrations + N+1 fix | [`migrations/mysql/`](migrations/mysql/), [`internal/listing/`](internal/listing/) | **measured, not estimated** — [`docs/performance/n-plus-one.md`](docs/performance/n-plus-one.md) |
| REST + gRPC | [`internal/app/routes.go`](internal/app/routes.go), [`api/`](api/) | OpenAPI spec + `.proto` files |
| 7 auth styles | [`internal/auth/`](internal/auth/) | JWT, OAuth2, OIDC, Basic, API-Key, Cookie+CSRF, SAML — see [`docs/architecture.md`](docs/architecture.md#auth-styles-mapped-to-real-endpoints) |
| Web security, hashing | [`internal/middleware/`](internal/middleware/), [`docs/security/owasp-top10-mapping.md`](docs/security/owasp-top10-mapping.md) | MD5 (ETag only), SHA-256, scrypt, bcrypt |
| Redis + HTTP caching | [`internal/cache/`](internal/cache/), [`internal/httputil/etag.go`](internal/httputil/etag.go) | cache-aside, explicit invalidation |
| Nginx | [`deployments/nginx/`](deployments/nginx/) | TLS termination, reverse proxy |
| CI/CD | [`.github/workflows/`](.github/workflows/) | lint, test, integration, e2e, Docker publish, **real kind deploy** |
| Transactions, ACID, normalization, failure modes, profiling | [`internal/db/`](internal/db/), [`docs/adr/`](docs/adr/), [`docs/erd/`](docs/erd/) | deadlock-retry demonstrated in a real integration test |
| Unit / integration / functional (e2e) testing | [`internal/*/​*_test.go`](internal/), [`test/e2e/`](test/e2e/) | integration + e2e use real `testcontainers-go` MySQL |
| Docker + Kubernetes | [`deployments/docker/`](deployments/docker/), [`deployments/k8s/`](deployments/k8s/) | manifests verified against a real `kind` cluster |
| Kafka + RabbitMQ | [`internal/eventbus/`](internal/eventbus/), [`internal/queue/`](internal/queue/) | distinct roles — see [ADR 0002](docs/adr/0002-cap-tradeoffs.md) |
| Elasticsearch | [`cmd/search-indexer/`](cmd/search-indexer/) | circuit breaker + MySQL fallback |
| MongoDB | [`internal/spec/`](internal/spec/), [`internal/damagereport/`](internal/damagereport/) | kept out of MySQL to preserve 3NF |
| Architectural patterns | [`docs/architecture.md`](docs/architecture.md#architectural-patterns-concretely) | monolith, microservices, serverless, service mesh, 12-factor |
| Real-time: SSE, WebSocket, polling | [`internal/realtime/`](internal/realtime/) | all three live-verified |
| DB scaling: indexes, replication, sharding, CAP | [`docs/performance/`](docs/performance/), [`docs/adr/`](docs/adr/) | replication verified against real containers |
| Resilience: degradation, throttling, back pressure, load shifting, circuit breaker | scattered — see [`docs/architecture.md`](docs/architecture.md#resilience-patterns) | |
| Observability | [`internal/observability/`](internal/observability/) | OTel traces, Prometheus metrics, structured logs |

## Testing

```bash
make test-unit          # go test ./... -short — no external dependencies
make test-integration   # spins up a real MySQL testcontainer; needs Docker
make test-e2e           # full HTTP booking lifecycle; needs Docker
```

## Explicitly thin-but-real treatments

Called out up front rather than discovered by a reviewer later:

- **SAML** ([`internal/auth/saml.go`](internal/auth/saml.go)): a real SP
  built with `crewjam/saml`, gated behind a configured IdP metadata URL —
  no live third-party IdP is wired into this repo by default.
- **Service mesh**: illustrative Istio-shaped YAML under
  [`deployments/k8s/mesh/`](deployments/k8s/mesh/), not a deployed control
  plane.
- **Sharding** ([`internal/availability/sharding.go`](internal/availability/sharding.go)):
  a real, unit-tested routing function, currently routing every shard
  index to one physical MySQL instance — see
  [ADR 0004](docs/adr/0004-sharding-strategy.md).
- **Serverless** ([`cmd/thumbnail-fn/`](cmd/thumbnail-fn/)): a real
  `aws-lambda-go` handler, exercised locally via a RabbitMQ-consumer
  entrypoint instead of a real AWS account.
- **sqlc** ([ADR 0001](docs/adr/0001-orm-choice.md)): the intended query
  generator, but this environment's Xcode Command Line Tools couldn't build
  its cgo dependency, so repositories are hand-written `sqlx` matching what
  `sqlc generate` would produce — `internal/db/queries/*.sql` is still the
  source of truth for when a working toolchain is available.

## Repository layout

See [`docs/architecture.md`](docs/architecture.md) for the full map;
briefly:

```
cmd/            4 binaries: api (monolith), notification-service,
                search-indexer, thumbnail-fn (dual Lambda/worker entrypoint)
internal/       one package per capability/concern
api/            .proto sources + generated gRPC code, OpenAPI spec
migrations/     golang-migrate SQL files (MySQL)
web/            plain HTML/CSS/JS frontend
deployments/    Docker, Nginx, Kubernetes, Prometheus/Grafana
docs/           ADRs, ERD, OWASP mapping, architecture, performance writeup
test/e2e/       full-stack HTTP tests
```
