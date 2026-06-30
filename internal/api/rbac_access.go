package api

import (
	"net/http"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/rbac"
)

func isMutation(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// authorizeOrg loads the caller's role in the org. It 403s if the caller is not
// a member, and (for mutating requests) if the role is read-only. Returns the
// role so handlers can apply finer checks.
func (s *Server) authorizeOrg(w http.ResponseWriter, r *http.Request, orgID int64) (string, bool) {
	u, _ := auth.UserFrom(r.Context())
	role, err := s.store.RoleInOrg(r.Context(), u.ID, orgID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of this org")
		return "", false
	}
	if isMutation(r.Method) && !rbac.CanWrite(role) {
		writeError(w, http.StatusForbidden, "read-only access (viewer role)")
		return "", false
	}
	return role, true
}

// requireOrgRole ensures the caller has at least the given role (regardless of
// HTTP method). Used for member-management endpoints.
func (s *Server) requireOrgRole(w http.ResponseWriter, r *http.Request, orgID int64, min string) (string, bool) {
	u, _ := auth.UserFrom(r.Context())
	role, err := s.store.RoleInOrg(r.Context(), u.ID, orgID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of this org")
		return "", false
	}
	if !rbac.AtLeast(role, min) {
		writeError(w, http.StatusForbidden, "requires "+min+" role")
		return "", false
	}
	return role, true
}
