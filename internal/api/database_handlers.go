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
	Name         string `json:"name"`
	Engine       string `json:"engine"`
	Tier         string `json:"tier"`
	Instances    int32  `json:"instances"`
	MinInstances int32  `json:"min_instances"`
	MaxInstances int32  `json:"max_instances"`
	Size         string `json:"size"`
	Storage      string `json:"storage"`
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
	tier := req.Tier
	if tier == "" {
		tier = "standard"
	}
	if tier != "standard" && tier != "autoscale" {
		writeError(w, http.StatusBadRequest, "tier must be standard or autoscale")
		return
	}
	if tier == "autoscale" && engine != "postgres" {
		writeError(w, http.StatusBadRequest, "autoscale tier is postgres-only")
		return
	}

	size, storage, ok := s.normalizeSizeStorage(w, req.Size, req.Storage)
	if !ok {
		return
	}
	// instances/min/max depend on tier.
	var instances, minI, maxI int32
	if tier == "autoscale" {
		minI, maxI = req.MinInstances, req.MaxInstances
		if minI < 1 {
			minI = 1
		}
		if maxI < minI {
			writeError(w, http.StatusBadRequest, "max_instances must be >= min_instances")
			return
		}
		instances = minI // start at the floor
	} else {
		instances = req.Instances
		if instances < 1 {
			instances = 1
		}
		if engine == "redis" && instances != 1 {
			writeError(w, http.StatusBadRequest, "redis supports a single instance")
			return
		}
		minI, maxI = instances, instances
	}

	db, err := s.store.CreateDatabase(r.Context(), projectID, req.Name, slugify(req.Name), engine, tier, instances, minI, maxI, size, storage)
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
	size, storage, ok := s.normalizeSizeStorage(w, req.Size, req.Storage)
	if !ok {
		return
	}
	instances := req.Instances
	if instances < 1 {
		instances = 1
	}
	if db.Engine == "redis" && instances != 1 {
		writeError(w, http.StatusBadRequest, "redis supports a single instance")
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

// normalizeSizeStorage validates and defaults size/storage inputs.
func (s *Server) normalizeSizeStorage(w http.ResponseWriter, size, storage string) (string, string, bool) {
	if size == "" {
		size = instance.Small
	}
	if !instance.IsValid(size) {
		writeError(w, http.StatusBadRequest, "invalid size (small|medium|large)")
		return "", "", false
	}
	if storage == "" {
		storage = "1Gi"
	}
	return size, storage, true
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
