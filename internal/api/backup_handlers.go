package api

import (
	"net/http"
	"strconv"

	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

// handleTriggerBackup requests an on-demand backup (controller performs it).
func (s *Server) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	if !db.Backups {
		writeError(w, http.StatusConflict, "backups are not enabled for this database")
		return
	}
	if err := s.store.Notify(r.Context(), store.DatabaseBackupChannel, strconv.FormatInt(db.ID, 10)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not request backup")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleListBackups lists the database's backups.
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	db, orgID, ok := s.requireDatabaseAccess(w, r)
	if !ok {
		return
	}
	backups, err := s.reader.ListBackups(r.Context(), naming.NamespaceForOrg(orgID), naming.DatabaseCluster(db.Slug))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list backups")
		return
	}
	writeJSON(w, http.StatusOK, backups)
}
