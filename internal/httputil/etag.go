package httputil

import (
	"crypto/md5" //nolint:gosec // intentional: MD5 here is a fast, non-security checksum
	"encoding/hex"
	"net/http"
	"time"
)

// WriteCacheableJSON implements client-side HTTP caching (requirement:
// "HTTP caching for client side"): an ETag/Last-Modified/Cache-Control set on
// the response, checked against If-None-Match/If-Modified-Since so an
// unchanged GET /api/v1/listings returns a bodiless 304 instead of the full
// payload. This is deliberately separate from the Redis server-side
// cache-aside layer in internal/cache — the two solve different problems
// (skip re-computation vs. skip re-transmission).
//
// MD5 is used ONLY as a fast content-fingerprint here, never for anything
// security-sensitive (passwords use bcrypt: see internal/auth/password_bcrypt.go).
// MD5's collision-resistance break is irrelevant to an ETag's job — the worst
// case of a crafted collision is a wrongly-served 304, not a security bypass.
func WriteCacheableJSON(w http.ResponseWriter, r *http.Request, status int, lastModified time.Time, body []byte) {
	sum := md5.Sum(body) //nolint:gosec
	etag := `"` + hex.EncodeToString(sum[:]) + `"`

	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	w.Header().Set("Cache-Control", "private, max-age=30, must-revalidate")

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if since := r.Header.Get("If-Modified-Since"); since != "" {
		if t, err := http.ParseTime(since); err == nil && !lastModified.After(t) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
