package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/instance"
	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

type createDatabaseReq struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Instances int32  `json:"instances"`
	Size      string `json:"size"`
	Storage   string `json:"storage"`
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
	instances, size, storage, ok := s.normalizeDBScaling(w, engine, req.Instances, req.Size, req.Storage)
	if !ok {
		return
	}
	db, err := s.store.CreateDatabase(r.Context(), projectID, req.Name, slugify(req.Name), engine, instances, size, storage)
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

// handleDatabaseConnection returns the connection URIs once the database is
// ready. URIs contain credentials, so they are only exposed here. read_uri is
// present only for HA databases (a load-balanced read-only endpoint).
func (s *Server) handleDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	if db.Status != store.DBStatusReady || db.ConnectionURI == "" {
		writeError(w, http.StatusConflict, "database not ready")
		return
	}
	resp := map[string]string{"uri": db.ConnectionURI}
	if db.ReadURI != "" {
		resp["read_uri"] = db.ReadURI
	}
	writeJSON(w, http.StatusOK, resp)
}

type scaleDatabaseReq struct {
	Instances int32  `json:"instances"`
	Size      string `json:"size"`
	Storage   string `json:"storage"`
}

// handleScaleDatabase changes a database's instances/size/storage and triggers
// a re-reconcile. Only Postgres supports more than one instance.
func (s *Server) handleScaleDatabase(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	var req scaleDatabaseReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Size == "" {
		req.Size = db.Size
	}
	if req.Storage == "" {
		req.Storage = db.Storage
	}
	if req.Instances == 0 {
		req.Instances = db.Instances
	}
	instances, size, storage, ok := s.normalizeDBScaling(w, db.Engine, req.Instances, req.Size, req.Storage)
	if !ok {
		return
	}
	if err := s.store.SetDatabaseScaling(r.Context(), db.ID, instances, size, storage); err != nil {
		writeError(w, http.StatusInternalServerError, "could not scale database")
		return
	}
	if err := s.store.Notify(r.Context(), store.DatabaseChannel, strconv.FormatInt(db.ID, 10)); err != nil {
		s.log.Warn("notify database scale failed", "database_id", db.ID, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// normalizeDBScaling validates and defaults scaling inputs. Redis is limited to
// a single instance (replicas need real clustering).
func (s *Server) normalizeDBScaling(w http.ResponseWriter, engine string, instances int32, size, storage string) (int32, string, string, bool) {
	if size == "" {
		size = instance.Small
	}
	if !instance.IsValid(size) {
		writeError(w, http.StatusBadRequest, "invalid size (small|medium|large)")
		return 0, "", "", false
	}
	if storage == "" {
		storage = "1Gi"
	}
	if instances < 1 {
		instances = 1
	}
	if engine == "redis" && instances != 1 {
		writeError(w, http.StatusBadRequest, "redis supports a single instance")
		return 0, "", "", false
	}
	return instances, size, storage, true
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
