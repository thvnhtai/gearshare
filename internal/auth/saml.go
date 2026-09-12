package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/crewjam/saml/samlsp"
)

// SAMLServiceProvider is GearShare's seventh and last auth style: SSO for a
// fictitious B2B "gear rental for businesses" admin portal
// (/business/admin/*), where a customer's own IdP — not GearShare — is the
// source of truth for who's allowed to log in. This is a genuinely
// different trust model from every other auth style here: GearShare never
// sees a password, only a signed assertion from an IdP it has been told,
// out of band, to trust.
//
// crewjam/saml is used against IdP metadata supplied via config
// (SAML_IDP_METADATA_URL) rather than a hardcoded provider — in local dev,
// that should point at a locally-run test IdP (e.g. crewjam/saml's own
// samlidp command, or Keycloak's SAML realm export) rather than a real
// enterprise IdP; see docs/architecture.md's auth-style table and the
// project plan's "explicitly thin-but-real treatments" for why a live
// third-party IdP isn't wired up in this repo directly.
type SAMLServiceProvider struct {
	middleware *samlsp.Middleware
}

// NewSAMLServiceProvider builds the SP from a self-signed cert/key pair
// (scripts/gen-dev-certs.sh generates one for local use — see
// deployments/idp/saml/) and the IdP's published metadata URL.
func NewSAMLServiceProvider(rootURL string, idpMetadataURL string, cert *x509.Certificate, key *rsa.PrivateKey) (*SAMLServiceProvider, error) {
	root, err := url.Parse(rootURL)
	if err != nil {
		return nil, fmt.Errorf("saml: parse root url: %w", err)
	}
	idpURL, err := url.Parse(idpMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("saml: parse idp metadata url: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(nil, http.DefaultClient, *idpURL)
	if err != nil {
		return nil, fmt.Errorf("saml: fetch idp metadata: %w", err)
	}

	mw, err := samlsp.New(samlsp.Options{
		URL:               *root,
		Key:               key,
		Certificate:       cert,
		IDPMetadata:       idpMetadata,
		AllowIDPInitiated: false, // SP-initiated only — a stricter, more auditable flow
	})
	if err != nil {
		return nil, fmt.Errorf("saml: build service provider: %w", err)
	}
	return &SAMLServiceProvider{middleware: mw}, nil
}

// GenerateSelfSignedCert produces a throwaway SP signing/encryption
// keypair for local dev — a real deployment would use a certificate issued
// through the org's own PKI, not a self-signed one generated at boot.
func GenerateSelfSignedCert() (*rsa.PrivateKey, *x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: generate key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "GearShare SAML SP (dev)"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: create certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: parse certificate: %w", err)
	}
	return key, cert, nil
}

// MetadataHandler serves GET /saml/metadata — the SP metadata document the
// IdP administrator uploads/registers when setting up trust.
func (s *SAMLServiceProvider) MetadataHandler() http.Handler {
	return s.middleware
}

// ACSHandler serves POST /saml/acs — the Assertion Consumer Service
// endpoint the IdP redirects the browser back to with a signed assertion.
func (s *SAMLServiceProvider) ACSHandler() http.Handler {
	return s.middleware
}

// RequireSession guards /business/admin/* — a completed SAML login sets a
// session the same samlsp middleware validates on subsequent requests.
func (s *SAMLServiceProvider) RequireSession(next http.Handler) http.Handler {
	return s.middleware.RequireAccount(next)
}
