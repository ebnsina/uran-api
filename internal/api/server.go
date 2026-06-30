// Package api wires HTTP routes to the store and auth layers.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/k8s"
	"github.com/ebnsina/uran-api/internal/store"
)

// Server holds dependencies shared by the HTTP handlers.
type Server struct {
	store         *store.Store
	auth          *auth.Authenticator
	log           *slog.Logger
	webhookSecret string
	reader        *k8s.Reader
}

// New builds a Server. webhookSecret is the HMAC secret used to verify GitHub
// webhooks; reader provides read-only cluster access for logs/metrics.
func New(s *store.Store, a *auth.Authenticator, log *slog.Logger, webhookSecret string, reader *k8s.Reader) *Server {
	return &Server{store: s, auth: a, log: log, webhookSecret: webhookSecret, reader: reader}
}

// Router returns the configured HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealth)

	// Public auth endpoints.
	r.Post("/v1/auth/register", s.handleRegister)
	r.Post("/v1/auth/login", s.handleLogin)

	// GitHub webhook (authenticated by HMAC signature, not a session).
	r.Post("/v1/webhooks/github", s.handleGitHubWebhook)

	// Authenticated endpoints.
	r.Group(func(r chi.Router) {
		r.Use(s.auth.Middleware)
		r.Use(s.auditMiddleware)
		r.Post("/v1/auth/logout", s.handleLogout)
		r.Get("/v1/me", s.handleMe)
		r.Get("/v1/audit", s.handleListAudit)

		r.Get("/v1/tokens", s.handleListTokens)
		r.Post("/v1/tokens", s.handleCreateToken)
		r.Delete("/v1/tokens/{tokenID}", s.handleDeleteToken)

		r.Get("/v1/orgs", s.handleListOrgs)
		r.Post("/v1/orgs", s.handleCreateOrg)

		r.Get("/v1/orgs/{orgID}/registry-credentials", s.handleListRegistryCreds)
		r.Post("/v1/orgs/{orgID}/registry-credentials", s.handleAddRegistryCred)
		r.Delete("/v1/orgs/{orgID}/registry-credentials/{credID}", s.handleDeleteRegistryCred)

		r.Get("/v1/orgs/{orgID}/members", s.handleListMembers)
		r.Post("/v1/orgs/{orgID}/members", s.handleAddMember)
		r.Patch("/v1/orgs/{orgID}/members/{userID}", s.handleSetMemberRole)
		r.Delete("/v1/orgs/{orgID}/members/{userID}", s.handleRemoveMember)

		r.Get("/v1/orgs/{orgID}/projects", s.handleListProjects)
		r.Post("/v1/orgs/{orgID}/projects", s.handleCreateProject)

		r.Get("/v1/projects/{projectID}/services", s.handleListServices)
		r.Post("/v1/projects/{projectID}/services", s.handleCreateService)

		r.Get("/v1/services/{serviceID}/deploys", s.handleListDeploys)
		r.Post("/v1/services/{serviceID}/deploys", s.handleCreateDeploy)
		r.Post("/v1/services/{serviceID}/image-deploys", s.handleImageDeploy)
		r.Get("/v1/services/{serviceID}/runtime-logs", s.handleRuntimeLogs)
		r.Get("/v1/services/{serviceID}/metrics", s.handleMetrics)
		r.Get("/v1/deploys/{deployID}", s.handleGetDeploy)
		r.Get("/v1/deploys/{deployID}/logs", s.handleDeployLogs)
		r.Post("/v1/deploys/{deployID}/rollback", s.handleRollback)

		r.Get("/v1/services/{serviceID}/env", s.handleListEnv)
		r.Post("/v1/services/{serviceID}/env", s.handleSetEnv)
		r.Delete("/v1/services/{serviceID}/env/{key}", s.handleDeleteEnv)

		r.Get("/v1/services/{serviceID}/domains", s.handleListDomains)
		r.Post("/v1/services/{serviceID}/domains", s.handleAddDomain)
		r.Delete("/v1/services/{serviceID}/domains/{domain}", s.handleDeleteDomain)

		r.Post("/v1/services/{serviceID}/scale", s.handleScale)
		r.Post("/v1/services/{serviceID}/health", s.handleSetHealth)
		r.Post("/v1/services/{serviceID}/disk", s.handleAttachDisk)
		r.Delete("/v1/services/{serviceID}/disk", s.handleDetachDisk)
		r.Post("/v1/services/{serviceID}/suspend", s.handleSuspend)
		r.Post("/v1/services/{serviceID}/resume", s.handleResume)

		r.Get("/v1/projects/{projectID}/databases", s.handleListDatabases)
		r.Post("/v1/projects/{projectID}/databases", s.handleCreateDatabase)
		r.Get("/v1/databases/{databaseID}", s.handleGetDatabase)
		r.Get("/v1/databases/{databaseID}/connection", s.handleDatabaseConnection)
		r.Post("/v1/databases/{databaseID}/scale", s.handleScaleDatabase)
		r.Delete("/v1/databases/{databaseID}", s.handleDeleteDatabase)
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
