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
