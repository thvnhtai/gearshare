package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCVerifier verifies a Google-issued id_token's signature, issuer,
// audience, and expiry — the "who is this" identity layer that sits beside
// (not underneath) oauth_google.go's "is this authorized" access-token
// exchange. See that file's doc comment for why these are kept as two
// distinct code paths despite Google returning both from one endpoint.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func NewOIDCVerifier(ctx context.Context, clientID string) (*OIDCVerifier, error) {
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("oidc: discover provider: %w", err)
	}
	return &OIDCVerifier{verifier: provider.Verifier(&oidc.Config{ClientID: clientID})}, nil
}

// IdentityClaims is the subset of the verified id_token this app actually
// trusts to identify a user — deliberately not the whole claim set.
type IdentityClaims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// VerifyIDToken checks the token's signature against Google's published
// JWKS, its issuer, its audience (must match our client ID), and its
// expiry — any failure here means the token is untrustworthy, full stop,
// regardless of what claims it appears to contain.
func (v *OIDCVerifier) VerifyIDToken(ctx context.Context, rawIDToken string) (*IdentityClaims, error) {
	idToken, err := v.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify id_token: %w", err)
	}

	var claims IdentityClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc: decode claims: %w", err)
	}
	if !claims.EmailVerified {
		return nil, fmt.Errorf("oidc: email not verified by identity provider")
	}
	return &claims, nil
}
