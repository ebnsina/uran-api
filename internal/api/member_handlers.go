package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/rbac"
	"github.com/ebnsina/uran-api/internal/store"
)

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return
	}
	members, err := s.store.ListOrgMembers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

type addMemberReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return
	}
	callerRole, ok := s.requireOrgRole(w, r, orgID, rbac.Admin)
	if !ok {
		return
	}
	var req addMemberReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	role := req.Role
	if role == "" {
		role = rbac.Member
	}
	user, err := s.store.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		writeError(w, http.StatusNotFound, "no user with that email")
		return
	}
	if !s.applyRole(w, r, orgID, user.ID, role, callerRole) {
		return
	}
	writeJSON(w, http.StatusCreated, store.OrgMember{UserID: user.ID, Email: user.Email, Name: user.Name, Role: role})
}

type setRoleReq struct {
	Role string `json:"role"`
}

func (s *Server) handleSetMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := s.memberParams(w, r)
	if !ok {
		return
	}
	callerRole, ok := s.requireOrgRole(w, r, orgID, rbac.Admin)
	if !ok {
		return
	}
	var req setRoleReq
	if err := readJSON(r, &req); err != nil || !rbac.IsValid(req.Role) {
		writeError(w, http.StatusBadRequest, "valid role required")
		return
	}
	if !s.applyRole(w, r, orgID, userID, req.Role, callerRole) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyRole validates and persists a member's role, enforcing: only owners may
// grant admin/owner; only owners may modify an existing owner; and the last
// owner cannot be demoted. Returns false (after writing an error) on rejection.
func (s *Server) applyRole(w http.ResponseWriter, r *http.Request, orgID, userID int64, newRole, callerRole string) bool {
	if !s.canGrantRole(w, callerRole, newRole) {
		return false
	}
	current, err := s.store.RoleInOrg(r.Context(), userID, orgID)
	if err == nil && current == rbac.Owner && callerRole != rbac.Owner {
		writeError(w, http.StatusForbidden, "only an owner can modify an owner")
		return false
	}
	if err := s.guardLastOwner(r, orgID, userID, newRole); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return false
	}
	if err := s.store.UpsertOrgMember(r.Context(), orgID, userID, newRole); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update member role")
		return false
	}
	return true
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := s.memberParams(w, r)
	if !ok {
		return
	}
	if _, ok := s.requireOrgRole(w, r, orgID, rbac.Admin); !ok {
		return
	}
	if err := s.guardLastOwner(r, orgID, userID, ""); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	deleted, err := s.store.RemoveOrgMember(r.Context(), orgID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove member")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// memberParams resolves {orgID} and {userID} from the path.
func (s *Server) memberParams(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return 0, 0, false
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return 0, 0, false
	}
	return orgID, userID, true
}

// canGrantRole enforces that only owners may grant admin or owner roles.
func (s *Server) canGrantRole(w http.ResponseWriter, callerRole, targetRole string) bool {
	if rbac.AtLeast(targetRole, rbac.Admin) && callerRole != rbac.Owner {
		writeError(w, http.StatusForbidden, "only an owner can grant admin/owner")
		return false
	}
	return true
}

// guardLastOwner rejects changes that would leave the org with no owner.
// newRole == "" means the member is being removed.
func (s *Server) guardLastOwner(r *http.Request, orgID, userID int64, newRole string) error {
	current, err := s.store.RoleInOrg(r.Context(), userID, orgID)
	if err != nil || current != rbac.Owner {
		return nil // target isn't an owner (or not a member); nothing to guard
	}
	if newRole == rbac.Owner {
		return nil // still an owner
	}
	owners, err := s.store.CountOrgOwners(r.Context(), orgID)
	if err != nil {
		return nil
	}
	if owners <= 1 {
		return errors.New("cannot remove or demote the last owner")
	}
	return nil
}
