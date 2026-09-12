# Twelve-Factor App — mapped to this repo

| Factor | GearShare's implementation |
|---|---|
| I. Codebase | One monorepo, one `go.mod` (`github.com/thvnhtai/gearshare`), multiple deploys (`cmd/api`, `cmd/notification-service`, `cmd/search-indexer`, `cmd/thumbnail-fn`) from the same codebase. |
| II. Dependencies | `go.mod`/`go.sum` declare and pin every dependency explicitly; no reliance on a system-installed library beyond the Go toolchain itself. |
| III. Config | Everything environment-driven via [`internal/config/config.go`](../internal/config/config.go) — DSNs, secrets, feature toggles, ports. `.env.example` documents the shape; `.env` is gitignored. No config file is baked into a container image. |
| IV. Backing services | MySQL, Redis, MongoDB, Kafka, RabbitMQ, and Elasticsearch are all attached as resources via config (host/port/URL), swappable without a code change — e.g. `MYSQL_PRIMARY_DSN` can point at a managed RDS instance in production with zero code difference from local docker-compose. |
| V. Build, release, run | GitHub Actions (`ci.yml`) builds and tests; `docker-publish.yml` produces an immutable, tagged image (the release); `deploy-kind.yml`/the k8s manifests run that exact image — the running process never rebuilds itself. |
| VI. Processes | `cmd/api` keeps no in-memory state that would break horizontal scaling *except* the real-time `Hub` (`internal/realtime/hub.go`), which is explicitly documented as a known single-process limitation (see its doc comment) rather than pretended away — sessions/JWTs are stateless, and cache-aside state lives in Redis, not in-process. |
| VII. Port binding | The API is fully self-contained and binds its own port (`HTTP_ADDR`, `GRPC_ADDR`) — it doesn't rely on Apache/IIS injection. Nginx in front of it is a reverse proxy, not a runtime host. |
| VIII. Concurrency | Scale-out is by process: run more `cmd/api` replicas behind Nginx/a k8s Service. `notification-service` and `search-indexer` scale independently along the same model (`deployments/k8s/base`'s per-service Deployments + HPA). |
| IX. Disposability | Graceful shutdown on `SIGINT`/`SIGTERM` (`cmd/api/main.go`'s signal handling + `app.Server.Shutdown`'s bounded-timeout drain); booking creation's transaction-or-nothing semantics (`internal/booking/tx.go`) mean a killed process mid-request never leaves a half-written booking. |
| X. Dev/prod parity | The same `docker-compose.yml` stack (MySQL, Redis, Kafka, RabbitMQ, Elasticsearch, Mongo, Jaeger, Prometheus) runs locally as in CI's integration-test job — no SQLite-in-dev/MySQL-in-prod split. |
| XI. Logs | Structured logs to stdout only (`internal/observability/logger.go`, `slog`) — no log files, no in-process log rotation. Correlation/trace IDs are attached so a log stream can be aggregated externally (Loki/CloudWatch/whatever the deploy target uses) without the app knowing or caring. |
| XII. Admin processes | `cmd/migrate`-equivalent is `golang-migrate`'s CLI run via `make migrate-up`/`migrate-down` against the same `MYSQL_PRIMARY_DSN` config the app itself uses — one-off admin tasks share the app's config and dependency versions rather than drifting. |
