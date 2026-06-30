package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/store"
)

type createTokenReq struct {
	Name string `json:"name"`
}

type createTokenResp struct {
	store.APIToken
	Token string `json:"token"` // shown once, at creation
}

// handleCreateToken issues a new personal access token. The plaintext token is
// returned only here; afterward only its prefix is visible.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var req createTokenReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	token, err := auth.NewAPIToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate token")
		return
	}
	prefix := token[:len(auth.APITokenPrefix)+8]
	t, err := s.store.CreateAPIToken(r.Context(), u.ID, req.Name, auth.HashAPIToken(token), prefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create token")
		return
	}
	writeJSON(w, http.StatusCreated, createTokenResp{APIToken: t, Token: token})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	tokens, err := s.store.ListAPITokens(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tokens")
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "tokenID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token id")
		return
	}
	deleted, err := s.store.DeleteAPIToken(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete token")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
