# OWASP Top 10 (2021) — risk to mitigation to code

Each row names the concrete GearShare code that addresses it — not a
restatement of the OWASP description.

## A01: Broken Access Control
- JWT-scoped ownership checks: booking approve/reject/cancel/complete only
  make sense for the listing's owner or the booking's renter — enforced in
  [`internal/booking/service.go`](../../internal/booking/service.go)'s
  status-machine (`CanTransition`, `internal/booking/model.go`) plus the
  handler requiring `RequireJWT`.
- Least-privilege DB user: the app connects as `gearshare_app` (SELECT/
  INSERT/UPDATE/DELETE only, no DDL) — see
  [`migrations/mysql/000001_create_app_user.up.sql`](../../migrations/mysql/000001_create_app_user.up.sql).
  A SQL-injection bug (mitigated separately below) can't `DROP TABLE` even
  if it existed, because that user has no DDL grant.

## A02: Cryptographic Failures
- Passwords: bcrypt (cost 12) by default —
  [`internal/auth/password_bcrypt.go`](../../internal/auth/password_bcrypt.go).
  scrypt is implemented alongside for the tradeoff writeup
  ([`password_scrypt.go`](../../internal/auth/password_scrypt.go)) but is
  not the default.
- API keys and refresh tokens: never stored raw, only a SHA-256 digest
  ([`internal/auth/apikey.go`](../../internal/auth/apikey.go),
  [`jwt.go`](../../internal/auth/jwt.go)).
- MD5 appears exactly once in the codebase, for HTTP `ETag` generation
  ([`internal/httputil/etag.go`](../../internal/httputil/etag.go)) — a
  non-security content fingerprint, explicitly commented as unsafe for
  anything password-adjacent.
- TLS termination at Nginx with self-signed certs for local dev
  ([`deployments/nginx/nginx.conf`](../../deployments/nginx/nginx.conf),
  [`scripts/gen-dev-certs.sh`](../../scripts/gen-dev-certs.sh)).

## A03: Injection
- Every query in the codebase is a parameterized `?` placeholder via
  `database/sql`/`sqlx` — no string-concatenated SQL anywhere (grep for
  `fmt.Sprintf` near a query: the one hit, `internal/user/repository.go`'s
  `get()`, interpolates a column *name* from a fixed internal enum, never
  user input, into the query text — values are still bound as `?` args).
- `httputil.DecodeJSON` (`internal/httputil/response.go`) uses
  `DisallowUnknownFields`, rejecting requests smuggling unexpected fields.
- Restrictive CSP (`internal/middleware/csp.go`) as defense-in-depth against
  stored/reflected XSS reaching script execution.

## A04: Insecure Design
- The booking lifecycle is a closed state machine
  (`internal/booking/model.go`'s `validTransitions`), not free-form status
  strings — an "approved → requested" downgrade is a compile-time-checked
  impossibility, not just a missing runtime check.
- Double-booking is prevented by a row-locked transaction
  (`internal/booking/tx.go`), not an application-level "check then insert"
  race.

## A05: Security Misconfiguration
- CORS is an explicit origin allowlist from config, never `*`
  (`internal/middleware/cors.go`).
- CSP, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` set on
  every response (`internal/middleware/csp.go`).
- Secrets loaded from environment variables only
  (`internal/config/config.go`), never committed — `.env` is gitignored,
  `.env.example` documents the shape with placeholder values.
- Production explicitly refuses to boot with the default dev JWT secret
  (`config.Load`'s `APP_ENV == "production"` check).

## A06: Vulnerable and Outdated Components
- `go.mod`/`go.sum` pin exact dependency versions; GitHub Actions CI
  (`.github/workflows/ci.yml`) runs on every push, so a bumped dependency
  is exercised by the test suite before merge.

## A07: Identification and Authentication Failures
- Seven distinct, purpose-matched auth mechanisms rather than one
  mechanism stretched over every use case — see each endpoint's auth style
  in the REST inventory (`docs/architecture.md`). Using JWT for the public
  API but Basic Auth for `/internal/*` means a leaked frontend JWT secret
  and a leaked ops password are two separate blast radii.
- Login/register are rate-limited separately and more tightly than the
  general API (`internal/app/routes.go`: 20/min vs 300/min) to blunt
  credential-stuffing.

## A08: Software and Data Integrity Failures
- CI (`ci.yml`) runs `go test ./...` and `golangci-lint` before any image
  is built; `docker-publish.yml` only builds/pushes on a tag, from a commit
  that already passed CI.

## A09: Security Logging and Monitoring Failures
- Structured logging with request/trace IDs
  (`internal/observability/logger.go`) and OpenTelemetry spans across
  REST → gRPC → Kafka → consumer boundaries, so an auth failure or a
  booking-transaction retry is traceable end to end, not just a bare log
  line with no correlation.

## A10: Server-Side Request Forgery (SSRF)
- No user-supplied URL is ever fetched server-side. The OAuth2/OIDC/SAML
  flows only ever call fixed, config-defined provider endpoints
  (`internal/auth/oauth_google.go`, `oidc.go`, `saml.go`) — never a
  user-controlled redirect target for the outbound leg.
