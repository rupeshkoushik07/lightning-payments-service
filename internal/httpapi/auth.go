package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/rupeshkoushik07/ln-payment-go/internal/store"
)

const (
	sessionCookie = "session"
	sessionLife   = 8 * time.Hour
)

type UserStore interface {
	UserForLogin(ctx context.Context, username string) (store.User, string, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID int64, expires time.Time) error
	SessionUser(ctx context.Context, tokenHash []byte) (store.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// Auth answers two questions for every request:
// who are you (the session cookie), and may you do this (the role).
type Auth struct {
	Users        UserStore
	SecureCookie bool // true when served over HTTPS
}

// A hash to compare against when the username does not exist, so that
// "no such user" takes as long as "wrong password".
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("no such user"), bcrypt.DefaultCost)

// POST /api/login   body: {"username": "...", "password": "..."}
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, "body must be JSON with username and password")
		return
	}

	user, hash, err := a.Users.UserForLogin(r.Context(), body.Username)
	if errors.Is(err, store.ErrNotFound) {
		hash = string(dummyHash)
	} else if err != nil {
		reply(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	// One message for both mistakes: never reveal which usernames exist.
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil || errors.Is(err, store.ErrNotFound) {
		reply(w, http.StatusUnauthorized, "wrong username or password")
		return
	}

	if err := a.startSession(w, r, user); err != nil {
		reply(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username, "role": user.Role})
}

// startSession is the same for every way of signing in: once we know who the
// person is, give their browser a ticket.
// The cookie holds a long random value. The database holds only its hash.
func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(sessionLife)
	if err := a.Users.CreateSession(r.Context(), hashToken(token), user.ID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,                    // page scripts cannot read it
		SameSite: http.SameSiteStrictMode, // other sites cannot send it
		Secure:   a.SecureCookie,
	})
	return nil
}

// POST /api/logout
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.Users.DeleteSession(r.Context(), hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.SecureCookie})
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/me
func (a *Auth) Me(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "role": u.Role})
}

// Require lets a request through only if it carries a valid session and the
// user has one of the given roles. No roles given means "any signed-in user".
func (a *Auth) Require(next http.Handler, roles ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			reply(w, http.StatusUnauthorized, "please sign in")
			return
		}
		user, err := a.Users.SessionUser(r.Context(), hashToken(c.Value))
		if errors.Is(err, store.ErrNotFound) {
			reply(w, http.StatusUnauthorized, "please sign in")
			return
		}
		if err != nil {
			reply(w, http.StatusInternalServerError, "could not check the session")
			return
		}
		if len(roles) > 0 && !slices.Contains(roles, user.Role) {
			reply(w, http.StatusForbidden, "your account may not do this")
			return
		}
		// Changes must come from our own pages, not from another website.
		if r.Method != http.MethodGet && !sameOrigin(r) {
			reply(w, http.StatusForbidden, "request came from another site")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

type userKey struct{}

func UserFrom(ctx context.Context) store.User {
	u, _ := ctx.Value(userKey{}).(store.User)
	return u
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// sameOrigin reports whether the browser says the request came from our own site.
// Tools like curl send no Origin header; they hold the cookie themselves, so they pass.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

// HashPassword is used when creating users.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}
