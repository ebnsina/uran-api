package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/ebnsina/uran-api/internal/store"
)

// requireServiceAccess resolves {serviceID} and authorizes the caller's role.
func (s *Server) requireServiceAccess(w http.ResponseWriter, r *http.Request) (store.Service, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return store.Service{}, false
	}
	svc, orgID, err := s.store.ServiceByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return store.Service{}, false
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return store.Service{}, false
	}
	return svc, true
}

type createDeployReq struct {
	CommitSHA string `json:"commit_sha"`
}

// handleCreateDeploy manually triggers a deploy for a service (no Git push
// required). Useful for re-deploys and testing.
func (s *Server) handleCreateDeploy(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req createDeployReq
	_ = readJSON(r, &req) // body optional

	d, err := s.enqueueDeploy(r.Context(), svc.ID, req.CommitSHA)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create deploy")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

type imageDeployReq struct {
	Image string `json:"image"`
}

// handleImageDeploy deploys a prebuilt container image to a service, skipping
// the Git build (CI push-to-deploy).
func (s *Server) handleImageDeploy(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req imageDeployReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Image) == "" {
		writeError(w, http.StatusBadRequest, "image required")
		return
	}
	d, err := s.store.CreateImageDeploy(r.Context(), svc.ID, strings.TrimSpace(req.Image))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create deploy")
		return
	}
	if err := s.store.Notify(r.Context(), store.DeploymentChannel, strconv.FormatInt(d.ID, 10)); err != nil {
		s.log.Warn("notify image deploy failed", "deploy_id", d.ID, "err", err)
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleListDeploys(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	deploys, err := s.store.ListDeploys(r.Context(), svc.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list deploys")
		return
	}
	writeJSON(w, http.StatusOK, deploys)
}

// requireDeployAccess resolves {deployID} and verifies the requester belongs to
// the owning service's org.
func (s *Server) requireDeployAccess(w http.ResponseWriter, r *http.Request) (store.Deploy, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "deployID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid deploy id")
		return store.Deploy{}, false
	}
	d, err := s.store.DeployByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "deploy not found")
		return store.Deploy{}, false
	}
	_, orgID, err := s.store.ServiceByID(r.Context(), d.ServiceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "deploy not found")
		return store.Deploy{}, false
	}
	if _, ok := s.authorizeOrg(w, r, orgID); !ok {
		return store.Deploy{}, false
	}
	return d, true
}

func (s *Server) handleGetDeploy(w http.ResponseWriter, r *http.Request) {
	d, ok := s.requireDeployAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// enqueueDeploy creates a queued deploy and publishes its ID on the deploy
// channel so the builder can pick it up. A failed notification is logged but
// does not fail the request — the deploy row is the source of truth and a
// reconciler can sweep stuck queued deploys.
func (s *Server) enqueueDeploy(ctx context.Context, serviceID int64, commitSHA string) (store.Deploy, error) {
	d, err := s.store.CreateDeploy(ctx, serviceID, commitSHA)
	if err != nil {
		return store.Deploy{}, err
	}
	if err := s.store.Notify(ctx, store.DeployChannel, strconv.FormatInt(d.ID, 10)); err != nil {
		s.log.Warn("notify deploy failed", "deploy_id", d.ID, "err", err)
	}
	return d, nil
}

// enqueuePreview creates a queued preview deploy for a pull request and
// publishes it on the deploy channel for the builder.
func (s *Server) enqueuePreview(ctx context.Context, serviceID int64, commitSHA string, prNumber int) (store.Deploy, error) {
	d, err := s.store.CreatePreviewDeploy(ctx, serviceID, commitSHA, prNumber)
	if err != nil {
		return store.Deploy{}, err
	}
	if err := s.store.Notify(ctx, store.DeployChannel, strconv.FormatInt(d.ID, 10)); err != nil {
		s.log.Warn("notify preview deploy failed", "deploy_id", d.ID, "err", err)
	}
	return d, nil
}
