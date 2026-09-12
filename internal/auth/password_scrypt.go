package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// ScryptHasher exists to demonstrate the tradeoff against bcrypt
// (docs/adr — memory-hard KDF vs. bcrypt's fixed, small memory footprint):
// scrypt is more resistant to custom ASIC/GPU cracking because it deliberately
// costs RAM, not just CPU time, but that same memory cost makes it easier to
// misconfigure (too low = no protection, too high = DoS your own servers) and
// harder to horizontally tune under load than bcrypt's single cost factor.
// GearShare does NOT use this as the default — see password_bcrypt.go.
type ScryptHasher struct {
	N, R, P, KeyLen int
}

func NewScryptHasher() *ScryptHasher {
	// N=32768 (2^15), r=8, p=1 — interactive-login-friendly parameters per
	// the scrypt paper's guidance, keeping verify time in the ~50-100ms range.
	return &ScryptHasher{N: 32768, R: 8, P: 1, KeyLen: 32}
}

// Hash returns "$scrypt$N$r$p$<salt-b64>$<key-b64>", self-describing so a
// future parameter bump doesn't break verification of old hashes.
func (h *ScryptHasher) Hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("scrypt: read salt: %w", err)
	}

	key, err := scrypt.Key([]byte(password), salt, h.N, h.R, h.P, h.KeyLen)
	if err != nil {
		return "", fmt.Errorf("scrypt: derive key: %w", err)
	}

	return fmt.Sprintf("$scrypt$%d$%d$%d$%s$%s",
		h.N, h.R, h.P,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func (h *ScryptHasher) Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 7 || parts[1] != "scrypt" {
		return false
	}

	var n, r, p int
	if _, err := fmt.Sscanf(parts[2]+" "+parts[3]+" "+parts[4], "%d %d %d", &n, &r, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[6])
	if err != nil {
		return false
	}

	got, err := scrypt.Key([]byte(password), salt, n, r, p, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
