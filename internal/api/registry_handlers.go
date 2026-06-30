package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/rbac"
)

type addRegistryReq struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAddRegistryCred(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.requireOrgRole(w, r, orgID, rbac.Admin); !ok {
		return
	}
	var req addRegistryReq
	if err := readJSON(r, &req); err != nil ||
		strings.TrimSpace(req.Registry) == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "registry, username, password required")
		return
	}
	c, err := s.store.AddRegistryCredential(r.Context(), orgID, req.Registry, req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save credential")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleListRegistryCreds(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.requireOrgRole(w, r, orgID, rbac.Admin); !ok {
		return
	}
	creds, err := s.store.ListRegistryCredentials(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list credentials")
		return
	}
	writeJSON(w, http.StatusOK, creds) // passwords are json:"-"
}

func (s *Server) handleDeleteRegistryCred(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.requireOrgRole(w, r, orgID, rbac.Admin); !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "credID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid credential id")
		return
	}
	deleted, err := s.store.DeleteRegistryCredential(r.Context(), orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete credential")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "credential not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
