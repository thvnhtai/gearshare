package user

import (
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/db"
	"github.com/thvnhtai/gearshare/internal/httputil"
)

const oauthStateCookie = "gearshare_oauth_state"

// OAuthHandler wires the OAuth2 (authorization) + OIDC (identity)
// "Log in with Google" flow onto the user domain: GET
// /api/v1/auth/oauth/google starts it, GET .../callback finishes it. See
// internal/auth/oauth_google.go and oidc.go for why these are two distinct
// verification steps rather than one.
type OAuthHandler struct {
	oauth  *auth.GoogleOAuth
	oidc   *auth.OIDCVerifier
	db     *db.DB
	repo   *Repository
	issuer *auth.JWTIssuer
	secure bool
	// frontendRedirect is where the browser lands after a successful
	// login, with the issued JWT as a query param for the plain-JS
	// frontend to pick up (web/login.html has no server-rendered session
	// of its own — see docs/architecture.md's auth-style mapping).
	frontendRedirect string
}

func NewOAuthHandler(oauth *auth.GoogleOAuth, oidcVerifier *auth.OIDCVerifier, database *db.DB, repo *Repository, issuer *auth.JWTIssuer, secure bool, frontendRedirect string) *OAuthHandler {
	return &OAuthHandler{oauth: oauth, oidc: oidcVerifier, db: database, repo: repo, issuer: issuer, secure: secure, frontendRedirect: frontendRedirect}
}

func (h *OAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateState()
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not start oauth flow")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/", HttpOnly: true,
		Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: 300,
	})
	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

func (h *OAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || r.URL.Query().Get("state") != cookie.Value {
		httputil.Error(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	code := r.URL.Query().Get("code")
	token, err := h.oauth.Exchange(r.Context(), code)
	if err != nil {
		httputil.Error(w, http.StatusBadGateway, "oauth code exchange failed")
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		httputil.Error(w, http.StatusBadGateway, "no id_token in oauth response")
		return
	}
	claims, err := h.oidc.VerifyIDToken(r.Context(), rawIDToken)
	if err != nil {
		httputil.Error(w, http.StatusUnauthorized, "id_token verification failed")
		return
	}

	var userID int64
	txErr := h.db.WithinTransaction(r.Context(), func(tx *sqlx.Tx) error {
		id, upsertErr := h.repo.UpsertOAuthIdentity(r.Context(), tx, "google", claims.Subject, &User{
			Email:       claims.Email,
			DisplayName: claims.Name,
		})
		if upsertErr != nil {
			return upsertErr
		}
		userID = id
		return nil
	})
	if txErr != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not complete oauth login")
		return
	}

	u, err := h.repo.GetByID(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not load oauth user")
		return
	}
	accessToken, err := h.issuer.IssueAccessToken(u.ID, string(u.Role))
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	// Attributes must match the cookie set in Start (HttpOnly/Secure/SameSite)
	// or some browsers won't recognize this as clearing the same cookie.
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	http.Redirect(w, r, h.frontendRedirect+"?access_token="+accessToken, http.StatusFound)
}
