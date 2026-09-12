package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/thvnhtai/gearshare/internal/httputil"
)

type ctxKey string

const ctxKeyClaims ctxKey = "auth.claims"
const ctxKeyAPIKeyOwner ctxKey = "auth.apikey_owner"

// RequireJWT is the primary auth middleware for /api/v1/*: the frontend
// sends "Authorization: Bearer <access token>" after login/register.
func RequireJWT(issuer *JWTIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				httputil.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}

			claims, err := issuer.Verify(token)
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext retrieves the verified JWT claims set by RequireJWT.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ctxKeyClaims).(*Claims)
	return claims, ok
}

// RequireAPIKey guards /partner/v1/* — the token/API-key auth style for
// machine-to-machine partner integrations that have no login flow or
// session, distinct from the JWT used by the frontend. The raw key travels
// as "Authorization: ApiKey <key>"; only its SHA-256 digest (apikey.go)
// ever touches the database or this function.
func RequireAPIKey(repo *APIKeyRepository, keyManager *APIKeyManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			rawKey, ok := strings.CutPrefix(header, "ApiKey ")
			if !ok || rawKey == "" {
				httputil.Error(w, http.StatusUnauthorized, "missing API key")
				return
			}

			hash := keyManager.hash(rawKey)
			ownerLabel, err := repo.FindActiveByHash(r.Context(), hash)
			if err != nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid or revoked API key")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyAPIKeyOwner, ownerLabel)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func APIKeyOwnerFromContext(ctx context.Context) (string, bool) {
	owner, ok := ctx.Value(ctxKeyAPIKeyOwner).(string)
	return owner, ok
}

// RequireBasicAuth guards internal operational endpoints (health-detailed,
// pprof) — a deliberately different, simpler auth style than the public API,
// appropriate because these are operator-only, low-traffic, and not exposed
// through Nginx to the public internet in production.
func RequireBasicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			validUser := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
			validPass := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
			if !ok || !validUser || !validPass {
				w.Header().Set("WWW-Authenticate", `Basic realm="gearshare-internal"`)
				httputil.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
