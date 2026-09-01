package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	v1alpha1 "github.com/TaliaMarine/k8s-driller/pkg/apis/driller/v1alpha1"
)

const (
	stateCookieName    = "driller_oidc_state"
	verifierCookieName = "driller_oidc_verifier"
	oidcFlowCookieTTL  = 10 * time.Minute
)

// RoleStore is the subset of crdstore.Store the OIDC callback needs to
// resolve a returning user's role (SPECS.md §4.1 — default viewer unless a
// DrillerUserRole already exists).
type RoleStore interface {
	GetUserRoleBySubject(ctx context.Context, subject string) (*v1alpha1.DrillerUserRole, error)
}

// Authenticator drives the OIDC Authorization Code + PKCE flow (SPECS.md
// §4.1).
type Authenticator struct {
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	sessions     *SessionManager
	roles        RoleStore
}

func NewAuthenticator(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string, sessions *SessionManager, roles RoleStore) (*Authenticator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &Authenticator{
		provider: provider,
		oauth2Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		sessions: sessions,
		roles:    roles,
	}, nil
}

func randToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func setFlowCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(oidcFlowCookieTTL),
	})
}

func clearFlowCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
}

// LoginHandler redirects to the OIDC provider's authorization endpoint.
func (a *Authenticator) LoginHandler(w http.ResponseWriter, r *http.Request) {
	state, err := randToken()
	if err != nil {
		http.Error(w, "failed to start login", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()

	setFlowCookie(w, stateCookieName, state)
	setFlowCookie(w, verifierCookieName, verifier)

	http.Redirect(w, r, a.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// CallbackHandler completes the OIDC flow, resolves the user's role, and
// issues a session cookie.
func (a *Authenticator) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid OIDC state", http.StatusBadRequest)
		return
	}
	verifierCookie, err := r.Cookie(verifierCookieName)
	if err != nil {
		http.Error(w, "invalid OIDC flow", http.StatusBadRequest)
		return
	}
	clearFlowCookie(w, stateCookieName)
	clearFlowCookie(w, verifierCookieName)

	ctx := r.Context()
	token, err := a.oauth2Config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusUnauthorized)
		return
	}
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "id_token verification failed", http.StatusUnauthorized)
		return
	}

	var claims struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "invalid id_token claims", http.StatusUnauthorized)
		return
	}

	role := v1alpha1.RoleViewer
	if existing, err := a.roles.GetUserRoleBySubject(ctx, claims.Subject); err == nil && existing != nil {
		role = existing.Spec.Role
	}

	if err := a.sessions.Issue(w, claims.Subject, claims.Email, role); err != nil {
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// LogoutHandler clears the session cookie.
func (a *Authenticator) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	a.sessions.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}
