package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleOAuth implements the OAuth2 authorization-code flow for "Log in
// with Google" — GearShare's second auth style, for renters who'd rather
// not create a password. This is deliberately coded and used separately
// from oidc.go's id_token verification: OAuth2 answers "is this request
// authorized to act as this Google account" (the access token), while OIDC
// answers "who is this person" (the id_token's verified identity claims).
// Google's endpoint happens to return both in one response, but treating
// them as the same concept is exactly the confusion this project's
// checklist asks to demonstrate NOT making.
type GoogleOAuth struct {
	config *oauth2.Config
}

func NewGoogleOAuth(clientID, clientSecret, redirectURL string) *GoogleOAuth {
	return &GoogleOAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

// GenerateState returns a random, unguessable value to be stored in a
// short-lived cookie and compared against the callback's `state` parameter
// — the standard OAuth2 CSRF defense for the authorization-code flow.
func GenerateState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (g *GoogleOAuth) AuthCodeURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// Exchange trades the authorization code for a token set. The returned
// token's raw id_token (if present) is what oidc.go's Verifier checks —
// this function itself makes no identity claim, only an authorization one.
func (g *GoogleOAuth) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oauth: exchange code: %w", err)
	}
	return token, nil
}

// GoogleProfile is fetched via the access token as a fallback/supplement
// when a caller wants profile data without going through full OIDC
// verification (e.g. a display name for a UI, not an identity decision).
type GoogleProfile struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (g *GoogleOAuth) FetchProfile(ctx context.Context, token *oauth2.Token) (*GoogleProfile, error) {
	client := g.config.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("oauth: fetch profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("oauth: fetch profile: status %d: %s", resp.StatusCode, body)
	}

	var profile GoogleProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("oauth: decode profile: %w", err)
	}
	return &profile, nil
}
