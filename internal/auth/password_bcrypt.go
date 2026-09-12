package auth

import "golang.org/x/crypto/bcrypt"

// BcryptHasher is the default password hasher for user registration/login
// (see docs/security/owasp-top10-mapping.md A02 Cryptographic Failures).
// bcrypt is chosen over scrypt (password_scrypt.go) as the actual default:
// it has no memory-cost knob to misconfigure, its cost factor is trivial to
// tune as hardware improves, and its Go implementation is battle-tested.
type BcryptHasher struct {
	Cost int
}

func NewBcryptHasher(cost int) *BcryptHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{Cost: cost}
}

func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.Cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (h *BcryptHasher) Verify(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
