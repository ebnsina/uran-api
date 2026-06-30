package api

import (
	"context"
	"net/http"

	"github.com/ebnsina/uran-api/internal/auth"
)

// auditLimit caps how many recent entries the audit endpoint returns.
const auditLimit = 100

// statusRecorder wraps a ResponseWriter to capture the status code while
// preserving streaming (Flusher) for SSE/log endpoints.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// auditMiddleware records every authenticated mutating request (method, path,
// resulting status) for the acting user. Reads are not recorded.
func (s *Server) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		if !isMutation(r.Method) {
			return
		}
		u, ok := auth.UserFrom(r.Context())
		if !ok {
			return
		}
		// Detached + async so auditing never delays or fails the response.
		method, path, status := r.Method, r.URL.Path, rec.status
		go s.store.InsertAudit(context.Background(), u.ID, u.Email, method, path, status)
	})
}

// handleListAudit returns the caller's recent audited actions.
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	entries, err := s.store.ListAudit(r.Context(), u.ID, auditLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
