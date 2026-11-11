package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

const flowCookie = "google_flow"

type EmailUsers interface {
	UserByEmail(ctx context.Context, email string) (store.User, error)
}

// Google lets a person prove who they are at Google instead of with a password.
// Google tells us WHO they are. Our own users table still decides WHAT they may do.
type Google struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	Users    EmailUsers
	Auth     *Auth
}

// NewGoogle fetches Google's public settings and signing keys location.
func NewGoogle(ctx context.Context, clientID, clientSecret, publicURL string, users EmailUsers, auth *Auth) (*Google, error) {
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, err
	}
	return &Google{
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimRight(publicURL, "/") + "/auth/google/callback",
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		Users:    users,
		Auth:     auth,
	}, nil
}

// GET /auth/google/start – send the browser to Google.
func (g *Google) Start(w http.ResponseWriter, r *http.Request) {
	state, nonce, pkce := randomText(), randomText(), oauth2.GenerateVerifier()

	// Remember three one-time values in a short-lived cookie:
	//   state – proves the answer belongs to a sign-in this browser started
	//   nonce – proves Google's note was made for this sign-in, not replayed
	//   pkce  – proves the one collecting the note is the one who asked
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure follows COOKIE_SECURE: on under HTTPS, off only for http://localhost
		Name: flowCookie, Value: state + "." + nonce + "." + pkce,
		Path: "/auth/google", MaxAge: 600, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // must survive the trip back from Google
		Secure:   g.Auth.SecureCookie,
	})
	http.Redirect(w, r, g.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(pkce)), http.StatusFound)
}

// GET /auth/google/callback – Google sends the browser back here.
func (g *Google) Callback(w http.ResponseWriter, r *http.Request) {
	user, err := g.identify(r)
	// The one-time values are used up, whatever happened.
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Value: "", Path: "/auth/google", MaxAge: -1, // #nosec G124 -- Secure follows COOKIE_SECURE: on under HTTPS, off only for http://localhost
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: g.Auth.SecureCookie})

	switch {
	case errors.Is(err, errNotInvited):
		http.Redirect(w, r, "/?signin=not_allowed", http.StatusSeeOther)
	case err != nil:
		slog.Warn("google sign-in failed", "err", err)
		http.Redirect(w, r, "/?signin=failed", http.StatusSeeOther)
	default:
		if err := g.Auth.startSession(w, r, user); err != nil {
			http.Redirect(w, r, "/?signin=failed", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

var errNotInvited = errors.New("this Google account has no user here")

func (g *Google) identify(r *http.Request) (store.User, error) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		return store.User{}, errors.New("no sign-in in progress")
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return store.User{}, errors.New("bad sign-in cookie")
	}
	state, nonce, pkce := parts[0], parts[1], parts[2]

	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) != 1 {
		return store.User{}, errors.New("state does not match")
	}
	if e := r.URL.Query().Get("error"); e != "" {
		return store.User{}, errors.New("google said: " + e)
	}

	// Swap the one-time code for Google's signed note (server to server).
	token, err := g.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(pkce))
	if err != nil {
		return store.User{}, err
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return store.User{}, errors.New("no id_token in Google's answer")
	}

	// Check the note: signed by Google, made for our app, not expired.
	idToken, err := g.verifier.Verify(r.Context(), raw)
	if err != nil {
		return store.User{}, err
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		return store.User{}, errors.New("nonce does not match")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return store.User{}, err
	}
	if !claims.EmailVerified || claims.Email == "" {
		return store.User{}, errors.New("google has not verified this email")
	}

	// Google told us who. Our table says whether they are one of ours, and their role.
	user, err := g.Users.UserByEmail(r.Context(), claims.Email)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, errNotInvited
	}
	return user, err
}

func randomText() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system has no randomness; nothing safe can be done
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
