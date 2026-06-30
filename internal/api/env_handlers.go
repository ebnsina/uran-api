package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/store"
)

type setEnvReq struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func (s *Server) handleSetEnv(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req setEnvReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Key) == "" {
		writeError(w, http.StatusBadRequest, "key required")
		return
	}
	if err := s.store.SetEnvVar(r.Context(), svc.ID, req.Key, req.Value, req.Secret); err != nil {
		writeError(w, http.StatusInternalServerError, "could not set env var")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListEnv(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	vars, err := s.store.ListEnvVars(r.Context(), svc.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list env vars")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) handleDeleteEnv(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	key := chi.URLParam(r, "key")
	deleted, err := s.store.DeleteEnvVar(r.Context(), svc.ID, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete env var")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "env var not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRollback creates a new deploy that reuses the target deploy's image,
// skipping the build and going straight to reconciliation. It also serves as
// the way to apply env-var changes without a rebuild.
func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	d, ok := s.requireDeployAccess(w, r)
	if !ok {
		return
	}
	if d.Image == "" {
		writeError(w, http.StatusConflict, "deploy has no built image to roll back to")
		return
	}
	nd, err := s.store.CreateRollbackDeploy(r.Context(), d.ServiceID, d.Image, d.CommitSHA, d.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create rollback deploy")
		return
	}
	if err := s.store.Notify(r.Context(), store.DeploymentChannel, strconv.FormatInt(nd.ID, 10)); err != nil {
		s.log.Warn("notify deployment failed", "deploy_id", nd.ID, "err", err)
	}
	writeJSON(w, http.StatusCreated, nd)
}
