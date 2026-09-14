package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ctxKeySessionUserID ctxKey = "auth.session_user_id"

// SessionManager implements cookie-based session auth for the
// server-rendered admin dashboard (/admin/dashboard/*) — deliberately
// distinct from the JWT bearer-token style used by the JSON API: the
// browser sends this back automatically via the Cookie header on every
// request to the same origin, which is exactly the ergonomics a classic
// server-rendered app wants and a JSON API explicitly avoids (to sidestep
// CSRF exposure on the API surface itself — see csrf.go for where that
// exposure gets re-introduced deliberately, here, and how it's mitigated).
//
// The session is a signed, stateless cookie (HMAC-SHA256 over
// "userID|expiryUnix"), not a server-side session store — a deliberate
// choice consistent with the twelve-factor "processes are stateless"
// requirement (docs/twelve-factor.md): any cmd/api replica can verify any
// session cookie without shared session storage.
type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

const SessionCookieName = "gearshare_admin_session"

func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	return &SessionManager{secret: []byte(secret), ttl: ttl}
}

func (m *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// IssueCookie sets the session cookie on the response. HttpOnly prevents
// any XSS payload from reading it via document.cookie; Secure is left to
// the caller (skipped on plain-HTTP local dev, enforced in production via
// Nginx-terminated HTTPS — see deployments/nginx/nginx.conf); SameSite=Lax
// blocks the cookie being sent on a cross-site top-level POST, which is
// most of what CSRF relies on before csrf.go's token check even runs.
func (m *SessionManager) IssueCookie(w http.ResponseWriter, userID int64, secure bool) {
	expiry := time.Now().Add(m.ttl).Unix()
	payload := fmt.Sprintf("%d|%d", userID, expiry)
	signature := m.sign(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signature

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiry, 0),
	})
}

// ClearCookie's attributes must match IssueCookie's (HttpOnly/Secure/SameSite)
// or some browsers won't recognize this as clearing the same cookie.
func (m *SessionManager) ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/admin", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// Verify returns the authenticated user ID from a request's session
// cookie, or an error if it's missing, malformed, forged, or expired.
func (m *SessionManager) Verify(r *http.Request) (int64, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return 0, fmt.Errorf("auth: no session cookie")
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("auth: malformed session cookie")
	}
	payloadRaw, signature := parts[0], parts[1]

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadRaw)
	if err != nil {
		return 0, fmt.Errorf("auth: malformed session payload")
	}
	payload := string(payloadBytes)

	expected := m.sign(payload)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return 0, fmt.Errorf("auth: invalid session signature")
	}

	fields := strings.SplitN(payload, "|", 2)
	if len(fields) != 2 {
		return 0, fmt.Errorf("auth: malformed session fields")
	}
	userID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("auth: malformed session user id")
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("auth: malformed session expiry")
	}
	if time.Now().Unix() > expiry {
		return 0, fmt.Errorf("auth: session expired")
	}

	return userID, nil
}

// RequireSession guards /admin/dashboard/*, redirecting to the login page
// on failure rather than returning a JSON 401 — the right UX for a
// browser-navigated, server-rendered surface.
func (m *SessionManager) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := m.Verify(r)
		if err != nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeySessionUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SessionUserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxKeySessionUserID).(int64)
	return id, ok
}
