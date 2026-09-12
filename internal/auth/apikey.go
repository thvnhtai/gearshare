package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// APIKeyManager issues and verifies opaque bearer API keys for the
// partner/integration surface (GET /partner/v1/*), a distinct auth style from
// end-user JWTs: no login flow, no expiry by default, one key per partner.
//
// Raw keys are never stored — only a SHA-256 digest, peppered with a
// server-side secret so a stolen DB dump alone can't be brute-forced offline
// the way a bare hash could. SHA-256 is appropriate here (unlike for
// passwords) because the key itself is already high-entropy random data, not
// something a human chose — there's no dictionary to defend against.
type APIKeyManager struct {
	pepper string
}

func NewAPIKeyManager(pepper string) *APIKeyManager {
	return &APIKeyManager{pepper: pepper}
}

// GeneratedKey is returned exactly once, at creation time, to the caller —
// only Hash is persisted.
type GeneratedKey struct {
	Raw  string
	Hash string
}

func (m *APIKeyManager) Generate() (*GeneratedKey, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("apikey: generate: %w", err)
	}
	raw := "gsk_" + base64.RawURLEncoding.EncodeToString(buf)
	return &GeneratedKey{Raw: raw, Hash: m.hash(raw)}, nil
}

func (m *APIKeyManager) Verify(raw, storedHash string) bool {
	computed := m.hash(raw)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

func (m *APIKeyManager) hash(raw string) string {
	sum := sha256.Sum256([]byte(raw + m.pepper))
	return hex.EncodeToString(sum[:])
}
