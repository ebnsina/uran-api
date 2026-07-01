package api

import (
	"fmt"
	"net/http"

	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
	"github.com/ebnsina/uran-api/internal/svctype"
)

// handleProjectStatus returns each service's latest deploy status.
func (s *Server) handleProjectStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.requireProjectAccess(w, r)
	if !ok {
		return
	}
	statuses, err := s.store.ProjectServiceStatuses(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load status")
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

type serviceDetail struct {
	store.Service
	URL          string `json:"url,omitempty"`           // public route: <slug>.<base-domain>
	InternalHost string `json:"internal_host,omitempty"` // in-cluster: <slug>.<namespace>
}

// handleGetService returns a service with its public URL and its in-cluster
// internal host (other services in the same project reach it at the latter).
func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	svc, orgID, ok := s.requireServiceWithOrg(w, r)
	if !ok {
		return
	}
	d := serviceDetail{Service: svc}
	if svctype.IsRoutable(svc.Type) {
		d.InternalHost = fmt.Sprintf("%s.%s", svc.Slug, naming.NamespaceForOrg(orgID))
		if s.baseDomain != "" {
			d.URL = fmt.Sprintf("%s.%s", svc.Slug, s.baseDomain)
		}
	}
	writeJSON(w, http.StatusOK, d)
}
