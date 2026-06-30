package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/store"
)

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type authResp struct {
	Token string     `json:"token"`
	User  store.User `json:"user"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email required and password must be at least 8 chars")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	u, err := s.store.CreateUser(r.Context(), req.Email, hash, req.Name)
	if err != nil {
		// Most likely a unique-violation on email.
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	s.issueSession(w, r.Context(), u)
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	u, err := s.store.UserByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(u.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.issueSession(w, r.Context(), u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); token != "" {
		_ = s.store.DeleteSession(r.Context(), strings.TrimSpace(token))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// issueSession creates a session token for the user and writes the auth response.
func (s *Server) issueSession(w http.ResponseWriter, ctx context.Context, u store.User) {
	token, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	expires := time.Now().Add(s.auth.TTL())
	if err := s.store.CreateSession(ctx, token, u.ID, expires); err != nil {
		writeError(w, http.StatusInternalServerError, "could not persist session")
		return
	}
	writeJSON(w, http.StatusCreated, authResp{Token: token, User: u})
}
