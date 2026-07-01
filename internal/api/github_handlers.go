package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/ebnsina/uran-api/internal/git"
	"github.com/go-chi/chi/v5"
)

// githubEnabled reports whether the OAuth App is configured.
func (s *Server) githubEnabled() bool { return s.ghClientID != "" && s.ghClientSecret != "" }

func orgIDParam(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	return id, err == nil
}

// handleGithubStatus reports whether the org has a connected GitHub account,
// and returns the authorize URL to start the connect flow.
func (s *Server) handleGithubStatus(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDParam(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return
	}
	resp := map[string]any{"enabled": s.githubEnabled(), "connected": false}
	if s.githubEnabled() {
		resp["authorize_url"] = fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&scope=repo&state=%d",
			s.ghClientID, orgID)
	}
	conn, err := s.store.GithubConnection(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read connection")
		return
	}
	if conn != nil {
		resp["connected"] = true
		resp["account"] = conn.AccountLogin
	}
	writeJSON(w, http.StatusOK, resp)
}

type githubConnectReq struct {
	Code string `json:"code"`
}

// handleGithubConnect exchanges an OAuth code for a token and stores it.
func (s *Server) handleGithubConnect(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDParam(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return
	}
	if !s.githubEnabled() {
		writeError(w, http.StatusBadRequest, "GitHub is not configured")
		return
	}
	var req githubConnectReq
	if err := readJSON(r, &req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}
	token, err := git.ExchangeCode(r.Context(), s.ghClientID, s.ghClientSecret, req.Code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub authorization failed")
		return
	}
	login, err := git.AccountLogin(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read GitHub account")
		return
	}
	if err := s.store.SetGithubConnection(r.Context(), orgID, token, login); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save connection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "account": login})
}

// handleGithubRepos lists repos the connected account can deploy from.
func (s *Server) handleGithubRepos(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDParam(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return
	}
	conn, err := s.store.GithubConnection(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read connection")
		return
	}
	if conn == nil {
		writeError(w, http.StatusBadRequest, "GitHub is not connected")
		return
	}
	repos, err := git.ListRepos(r.Context(), conn.AccessToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not list repositories")
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

// handleGithubDisconnect removes the org's GitHub connection.
func (s *Server) handleGithubDisconnect(w http.ResponseWriter, r *http.Request) {
	orgID, ok := orgIDParam(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return
	}
	if err := s.store.DeleteGithubConnection(r.Context(), orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not disconnect")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
