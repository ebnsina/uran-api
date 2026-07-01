package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/store"
)

// Audit pagination bounds.
const (
	auditDefaultLimit = 20
	auditMaxLimit     = 100
)

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

// auditPage is the paginated envelope returned by the audit endpoint.
type auditPage struct {
	Items  []store.AuditEntry `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// handleListAudit returns a filtered, paginated page of the caller's audited
// actions. Supports ?q= (path search), ?method=, ?limit=, ?offset=.
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	q := r.URL.Query()

	limit := auditDefaultLimit
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > auditMaxLimit {
		limit = auditMaxLimit
	}
	offset := 0
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}

	entries, total, err := s.store.ListAudit(r.Context(), u.ID, store.AuditQuery{
		Search: q.Get("q"),
		Method: q.Get("method"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list audit log")
		return
	}
	writeJSON(w, http.StatusOK, auditPage{Items: entries, Total: total, Limit: limit, Offset: offset})
}
