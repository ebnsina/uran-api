package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/auth"
	"github.com/ebnsina/uran-api/internal/svctype"
)

// slugify produces a lowercase, dash-separated slug from a name.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func pathInt(r *http.Request, key string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

type createOrgReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var req createOrgReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	slug := slugify(req.Name)
	if slug == "" {
		writeError(w, http.StatusBadRequest, "name must contain alphanumeric characters")
		return
	}
	org, err := s.store.CreateOrg(r.Context(), u.ID, req.Name, slug)
	if err != nil {
		writeError(w, http.StatusConflict, "could not create org (slug may be taken)")
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	orgs, err := s.store.ListOrgs(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list orgs")
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

// requireOrgMember resolves the {orgID} path param and verifies membership.
func (s *Server) requireOrgMember(w http.ResponseWriter, r *http.Request) (int64, bool) {
	orgID, ok := pathInt(r, "orgID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid org id")
		return 0, false
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return 0, false
	}
	return orgID, true
}

type createProjectReq struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.requireOrgMember(w, r)
	if !ok {
		return
	}
	var req createProjectReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	p, err := s.store.CreateProject(r.Context(), orgID, req.Name, slugify(req.Name))
	if err != nil {
		writeError(w, http.StatusConflict, "could not create project (slug may be taken)")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.requireOrgMember(w, r)
	if !ok {
		return
	}
	projects, err := s.store.ListProjects(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list projects")
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// requireProjectAccess resolves {projectID} and verifies the user is a member
// of the owning org.
func (s *Server) requireProjectAccess(w http.ResponseWriter, r *http.Request) (int64, bool) {
	projectID, ok := pathInt(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return 0, false
	}
	p, err := s.store.ProjectByID(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, false
	}
	if _, ok := s.authorizeOrg(w, r, p.OrgID); !ok {
		return 0, false
	}
	return projectID, true
}

type createServiceReq struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	RepoURL  string `json:"repo_url"`
	Branch   string `json:"branch"`
	Schedule string `json:"schedule"`
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.requireProjectAccess(w, r)
	if !ok {
		return
	}
	var req createServiceReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	typ := req.Type
	if typ == "" {
		typ = svctype.Web
	}
	if !svctype.IsValid(typ) {
		writeError(w, http.StatusBadRequest, "invalid service type")
		return
	}
	if svctype.RequiresSchedule(typ) && strings.TrimSpace(req.Schedule) == "" {
		writeError(w, http.StatusBadRequest, "cron services require a schedule")
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}
	svc, err := s.store.CreateService(r.Context(), projectID, req.Name, slugify(req.Name), typ, req.RepoURL, branch, req.Schedule)
	if err != nil {
		writeError(w, http.StatusConflict, "could not create service (slug may be taken)")
		return
	}
	writeJSON(w, http.StatusCreated, svc)
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.requireProjectAccess(w, r)
	if !ok {
		return
	}
	services, err := s.store.ListServices(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list services")
		return
	}
	writeJSON(w, http.StatusOK, services)
}
