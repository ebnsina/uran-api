package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/ebnsina/uran-api/internal/instance"
	"github.com/ebnsina/uran-api/internal/store"
)

type scaleReq struct {
	Replicas     int32  `json:"replicas"`
	InstanceSize string `json:"instance_size"`
	MinReplicas  int32  `json:"min_replicas"`
	MaxReplicas  int32  `json:"max_replicas"`
}

func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req scaleReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.InstanceSize == "" {
		req.InstanceSize = svc.InstanceSize
	}
	if !instance.IsValid(req.InstanceSize) {
		writeError(w, http.StatusBadRequest, "invalid instance_size (small|medium|large)")
		return
	}
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	// Autoscaling is enabled when max_replicas > 0; validate the bounds.
	if req.MaxReplicas > 0 {
		if req.MinReplicas < 1 {
			req.MinReplicas = 1
		}
		if req.MaxReplicas < req.MinReplicas {
			writeError(w, http.StatusBadRequest, "max_replicas must be >= min_replicas")
			return
		}
	}
	if err := s.store.SetServiceScaling(r.Context(), svc.ID, req.Replicas, req.InstanceSize, req.MinReplicas, req.MaxReplicas); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update scaling")
		return
	}
	s.applyServiceChange(r.Context(), svc.ID)
	w.WriteHeader(http.StatusNoContent)
}

type healthReq struct {
	Path string `json:"path"`
}

func (s *Server) handleSetHealth(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceAccess(w, r)
	if !ok {
		return
	}
	var req healthReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.SetServiceHealth(r.Context(), svc.ID, req.Path); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update health check")
		return
	}
	s.applyServiceChange(r.Context(), svc.ID)
	w.WriteHeader(http.StatusNoContent)
}

// applyServiceChange re-applies the current settings without a rebuild by
// re-deploying the service's latest built image. If the service has never been
// deployed, the change takes effect on the next deploy.
func (s *Server) applyServiceChange(ctx context.Context, serviceID int64) {
	last, err := s.store.LatestImagedDeploy(ctx, serviceID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Warn("apply service change: lookup failed", "service_id", serviceID, "err", err)
		}
		return
	}
	nd, err := s.store.CreateRollbackDeploy(ctx, serviceID, last.Image, last.CommitSHA, last.ID)
	if err != nil {
		s.log.Warn("apply service change: redeploy failed", "service_id", serviceID, "err", err)
		return
	}
	if err := s.store.Notify(ctx, store.DeploymentChannel, strconv.FormatInt(nd.ID, 10)); err != nil {
		s.log.Warn("apply service change: notify failed", "deploy_id", nd.ID, "err", err)
	}
}
