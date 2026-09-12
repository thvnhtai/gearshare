// Package middleware holds cross-cutting HTTP middleware shared by every
// route group: CORS, CSP, rate limiting, request IDs, panic recovery,
// tracing, and the circuit breaker wrapper.
package middleware

import (
	"net/http"

	"github.com/go-chi/cors"
)

// CORS restricts cross-origin requests to the configured frontend origin(s)
// only — see docs/security/owasp-top10-mapping.md (A05 Security
// Misconfiguration: a wildcard "*" origin would let any website read
// authenticated responses via a victim's browser using stolen credentials).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
