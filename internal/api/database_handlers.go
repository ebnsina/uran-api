package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

type createDatabaseReq struct {
	Name   string `json:"name"`
	Engine string `json:"engine"`
}

func (s *Server) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.requireProjectAccess(w, r)
	if !ok {
		return
	}
	var req createDatabaseReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	engine := req.Engine
	if engine == "" {
		engine = "postgres"
	}
	if engine != "postgres" && engine != "redis" {
		writeError(w, http.StatusBadRequest, "unsupported engine (postgres|redis)")
		return
	}
	db, err := s.store.CreateDatabase(r.Context(), projectID, req.Name, slugify(req.Name), engine)
	if err != nil {
		writeError(w, http.StatusConflict, "could not create database (slug may be taken)")
		return
	}
	if err := s.store.Notify(r.Context(), store.DatabaseChannel, strconv.FormatInt(db.ID, 10)); err != nil {
		s.log.Warn("notify database failed", "database_id", db.ID, "err", err)
	}
	writeJSON(w, http.StatusCreated, db)
}

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.requireProjectAccess(w, r)
	if !ok {
		return
	}
	dbs, err := s.store.ListDatabases(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list databases")
		return
	}
	writeJSON(w, http.StatusOK, dbs)
}

// requireDatabaseAccess resolves {databaseID} and verifies org membership,
// returning the database and its org id.
func (s *Server) requireDatabaseAccess(w http.ResponseWriter, r *http.Request) (store.Database, int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "databaseID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid database id")
		return store.Database{}, 0, false
	}
	db, orgID, err := s.store.DatabaseByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "database not found")
		return store.Database{}, 0, false
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return store.Database{}, 0, false
	}
	return db, orgID, true
}

func (s *Server) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, db)
}

// handleDatabaseConnection returns the connection URI once the database is
// ready. The URI contains credentials, so it is only exposed here.
func (s *Server) handleDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	if db.Status != store.DBStatusReady || db.ConnectionURI == "" {
		writeError(w, http.StatusConflict, "database not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"uri": db.ConnectionURI})
}

func (s *Server) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	db, orgID, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	// Tell the controller to deprovision before removing the record.
	payload := naming.NamespaceForOrg(orgID) + ":" + naming.DatabaseCluster(db.Slug) + ":" + db.Engine
	if err := s.store.Notify(r.Context(), store.DatabaseTeardownChannel, payload); err != nil {
		s.log.Warn("notify database teardown failed", "database_id", db.ID, "err", err)
	}
	if err := s.store.DeleteDatabase(r.Context(), db.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete database")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
