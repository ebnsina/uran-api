package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/k8s"
	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

// requireServiceWithOrg resolves {serviceID}, verifies org membership, and
// returns the service plus its org id (needed to locate cluster objects).
func (s *Server) requireServiceWithOrg(w http.ResponseWriter, r *http.Request) (store.Service, int64, bool) {
	u, _ := auth.UserFrom(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return store.Service{}, 0, false
	}
	svc, orgID, err := s.store.ServiceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return store.Service{}, 0, false
	}
	member, err := s.store.IsOrgMember(r.Context(), u.ID, orgID)
	if err != nil || !member {
		writeError(w, http.StatusForbidden, "no access to this service")
		return store.Service{}, 0, false
	}
	return svc, orgID, true
}

// serviceSelector is the label selector matching a service's production pods.
func serviceSelector(slug string) string {
	return "app.kubernetes.io/name=" + slug
}

// handleRuntimeLogs streams the running pod's logs to the client (plain text,
// chunked). The connection ends when the stream closes or the client leaves.
func (s *Server) handleRuntimeLogs(w http.ResponseWriter, r *http.Request) {
	svc, orgID, ok := s.requireServiceWithOrg(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	err := s.reader.StreamLogs(r.Context(), naming.NamespaceForOrg(orgID), serviceSelector(svc.Slug), &flushWriter{w: w, f: flusher})
	if errors.Is(err, k8s.ErrNoPods) {
		_, _ = w.Write([]byte("no running pods for this service\n"))
		flusher.Flush()
	}
}

// handleMetrics returns current CPU/memory usage per pod for a service.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	svc, orgID, ok := s.requireServiceWithOrg(w, r)
	if !ok {
		return
	}
	metrics, err := s.reader.Metrics(r.Context(), naming.NamespaceForOrg(orgID), serviceSelector(svc.Slug))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "metrics unavailable")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// flushWriter flushes the HTTP response after every write so log lines stream
// live instead of buffering.
type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}
