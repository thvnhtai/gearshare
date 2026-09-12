package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRF implements the double-submit-cookie pattern for the cookie-session
// admin dashboard: a random token is set as a readable (non-HttpOnly)
// cookie, and every state-changing form must echo it back as a hidden
// field. A cross-site form can make the browser send the session cookie
// automatically, but it cannot read this cookie's value to put in its own
// form field (same-origin policy) — so a mismatch means the request didn't
// originate from a page GearShare itself rendered.
const CSRFCookieName = "gearshare_csrf_token"
const CSRFFormField = "csrf_token"

func GenerateCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func IssueCSRFCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/admin",
		HttpOnly: false, // must be JS/template-readable to echo into the form
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireCSRF guards POST routes under /admin/dashboard: the cookie value
// must match the submitted form field exactly.
func RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(CSRFCookieName)
		if err != nil {
			http.Error(w, "missing CSRF cookie", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		submitted := r.FormValue(CSRFFormField)

		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(submitted)) != 1 {
			http.Error(w, "CSRF token mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
