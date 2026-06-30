package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/store"
)

type addDomainReq struct {
	Domain string `json:"domain"`
}

func (s *Server) handleAddDomain(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req addDomainReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain required")
		return
	}
	d, err := s.store.AddCustomDomain(r.Context(), svc.ID, domain)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "domain already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not add domain")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	domains, err := s.store.ListCustomDomains(r.Context(), svc.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	domain := strings.ToLower(chi.URLParam(r, "domain"))
	deleted, err := s.store.DeleteCustomDomain(r.Context(), svc.ID, domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete domain")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
