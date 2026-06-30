// Package builder orchestrates the deploy build pipeline: it consumes queued
// deploys from the event bus, drives the deploy/build state machine, runs a
// build backend, and persists logs. It is the bridge between package store
// (state) and package build (image production).
package builder

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/ebnsina/uran-api/internal/build"
	"github.com/ebnsina/uran-api/internal/deploy"
	"github.com/ebnsina/uran-api/internal/store"
)

// Processor builds queued deploys.
type Processor struct {
	store    *store.Store
	backend  build.Builder
	registry string
	log      *slog.Logger
}

// New constructs a Processor. registry is the image registry host that built
// images are tagged for and pushed to (e.g. "localhost:5005").
func New(st *store.Store, backend build.Builder, registry string, log *slog.Logger) *Processor {
	return &Processor{store: st, backend: backend, registry: registry, log: log}
}

// Run listens on the deploy channel and processes each queued deploy in its own
// goroutine. It blocks until ctx is cancelled or the listen connection fails.
func (p *Processor) Run(ctx context.Context) error {
	p.log.Info("builder listening for deploys", "channel", store.DeployChannel)
	return p.store.Listen(ctx, store.DeployChannel, func(payload string) {
		id, err := strconv.ParseInt(payload, 10, 64)
		if err != nil {
			p.log.Warn("bad deploy notification", "payload", payload)
			return
		}
		go p.process(ctx, id)
	})
}

// process runs the full build pipeline for a single deploy. All terminal
// outcomes are recorded in the database; errors are logged, not returned,
// because there is no caller to handle them.
func (p *Processor) process(ctx context.Context, deployID int64) {
	log := p.log.With("deploy_id", deployID)

	d, err := p.store.DeployByID(ctx, deployID)
	if err != nil {
		log.Error("load deploy", "err", err)
		return
	}
	svc, _, err := p.store.ServiceByID(ctx, d.ServiceID)
	if err != nil {
		log.Error("load service", "err", err)
		return
	}
	b, err := p.store.BuildByDeployID(ctx, deployID)
	if err != nil {
		log.Error("load build", "err", err)
		return
	}

	image := fmt.Sprintf("%s/svc-%d:%d", p.registry, svc.ID, deployID)

	// queued -> building
	if _, err := p.store.UpdateDeployStatus(ctx, deployID, deploy.StatusBuilding); err != nil {
		log.Error("transition to building", "err", err)
		return
	}
	if err := p.store.StartBuild(ctx, b.ID, deploy.BuildRunning); err != nil {
		log.Error("start build", "err", err)
		return
	}

	logs := newDBLogWriter(ctx, p.store, b.ID)
	result, buildErr := p.backend.Build(ctx, build.Request{
		RepoURL: svc.RepoURL,
		Ref:     d.CommitSHA,
		Image:   image,
	}, logs)

	if buildErr != nil {
		log.Error("build failed", "err", buildErr)
		fmt.Fprintf(logs, "\nBUILD FAILED: %v\n", buildErr)
		p.fail(ctx, log, deployID, b.ID)
		return
	}

	if err := p.store.SetDeployImage(ctx, deployID, result.Image); err != nil {
		log.Error("set image", "err", err)
		p.fail(ctx, log, deployID, b.ID)
		return
	}
	if err := p.store.FinishBuild(ctx, b.ID, deploy.BuildSucceeded); err != nil {
		log.Error("finish build", "err", err)
	}
	// building -> deploying, then hand off to the controller via the event bus.
	if _, err := p.store.UpdateDeployStatus(ctx, deployID, deploy.StatusDeploying); err != nil {
		log.Error("transition to deploying", "err", err)
		return
	}
	if err := p.store.Notify(ctx, store.DeploymentChannel, strconv.FormatInt(deployID, 10)); err != nil {
		log.Warn("notify deployment failed", "err", err)
	}
	log.Info("build succeeded", "image", result.Image)
}

// fail records a failed build and deploy, best-effort.
func (p *Processor) fail(ctx context.Context, log *slog.Logger, deployID, buildID int64) {
	if err := p.store.FinishBuild(ctx, buildID, deploy.BuildFailed); err != nil {
		log.Error("mark build failed", "err", err)
	}
	if _, err := p.store.UpdateDeployStatus(ctx, deployID, deploy.StatusFailed); err != nil {
		log.Error("transition to failed", "err", err)
	}
}
