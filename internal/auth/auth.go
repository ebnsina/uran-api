// Package auth provides password hashing, session token generation, and the
// HTTP middleware that resolves a bearer token to the current user.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ebnsina/uran-api/internal/store"
)

// APITokenPrefix identifies personal access tokens (vs session tokens).
const APITokenPrefix = "uran_pat_"

// NewAPIToken returns a new personal access token string.
func NewAPIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return APITokenPrefix + hex.EncodeToString(b), nil
}

// HashAPIToken returns the SHA-256 hex digest stored for a token.
func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type ctxKey int

const userKey ctxKey = 0

// HashPassword returns a bcrypt hash for the given plaintext password.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether plain matches the stored bcrypt hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// NewToken returns a random, URL-safe session token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Authenticator resolves session tokens to users.
type Authenticator struct {
	store *store.Store
	ttl   time.Duration
}

// New creates an Authenticator.
func New(s *store.Store, ttl time.Duration) *Authenticator {
	return &Authenticator{store: s, ttl: ttl}
}

// TTL is the session lifetime.
func (a *Authenticator) TTL() time.Duration { return a.ttl }

// Middleware rejects requests without a valid bearer token (session or API
// token) and injects the authenticated user into the request context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := a.resolve(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolve authenticates a bearer token as either an API token or a session.
func (a *Authenticator) resolve(ctx context.Context, token string) (store.User, error) {
	if strings.HasPrefix(token, APITokenPrefix) {
		hash := HashAPIToken(token)
		u, err := a.store.UserByAPIToken(ctx, hash)
		if err == nil {
			a.store.TouchAPIToken(ctx, hash)
		}
		return u, err
	}
	return a.store.UserBySession(ctx, token)
}

// UserFrom returns the authenticated user from the context, if present.
func UserFrom(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey).(store.User)
	return u, ok
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}
