// Package controller consumes deploys whose image is built and reconciles them
// onto the cluster (advancing "deploying" -> "live"), and tears down preview
// environments when their pull request closes. It bridges package store (state)
// and package k8s (cluster reconciliation).
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/ebnsina/uran-api/internal/deploy"
	"github.com/ebnsina/uran-api/internal/k8s"
	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

// servicePort is the port Uran asks every service to listen on (exposed to the
// container as the PORT env var). Per-service ports can be added later.
const servicePort = 8080

// Processor reconciles built deploys onto the cluster.
type Processor struct {
	store *store.Store
	recon *k8s.Reconciler
	log   *slog.Logger
}

// New constructs a Processor.
func New(st *store.Store, recon *k8s.Reconciler, log *slog.Logger) *Processor {
	return &Processor{store: st, recon: recon, log: log}
}

// Run listens on the deployment and teardown channels concurrently until ctx is
// cancelled or a listen connection fails.
func (p *Processor) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		p.log.Info("controller listening for deployments", "channel", store.DeploymentChannel)
		return p.store.Listen(ctx, store.DeploymentChannel, func(payload string) {
			id, err := strconv.ParseInt(payload, 10, 64)
			if err != nil {
				p.log.Warn("bad deployment notification", "payload", payload)
				return
			}
			go p.reconcile(ctx, id)
		})
	})
	g.Go(func() error {
		p.log.Info("controller listening for teardowns", "channel", store.TeardownChannel)
		return p.store.Listen(ctx, store.TeardownChannel, func(payload string) {
			go p.teardown(ctx, payload)
		})
	})
	g.Go(func() error {
		p.log.Info("controller listening for databases", "channel", store.DatabaseChannel)
		return p.store.Listen(ctx, store.DatabaseChannel, func(payload string) {
			id, err := strconv.ParseInt(payload, 10, 64)
			if err != nil {
				p.log.Warn("bad database notification", "payload", payload)
				return
			}
			go p.reconcileDatabase(ctx, id)
		})
	})
	g.Go(func() error {
		p.log.Info("controller listening for database teardowns", "channel", store.DatabaseTeardownChannel)
		return p.store.Listen(ctx, store.DatabaseTeardownChannel, func(payload string) {
			go p.teardownDatabase(ctx, payload)
		})
	})
	return g.Wait()
}

// reconcile applies a deploy's workload to the cluster and records the outcome.
func (p *Processor) reconcile(ctx context.Context, deployID int64) {
	log := p.log.With("deploy_id", deployID)

	d, err := p.store.DeployByID(ctx, deployID)
	if err != nil {
		log.Error("load deploy", "err", err)
		return
	}
	if d.Image == "" {
		log.Error("deploy has no image; cannot reconcile")
		p.fail(ctx, log, deployID)
		return
	}
	svc, orgID, err := p.store.ServiceByID(ctx, d.ServiceID)
	if err != nil {
		log.Error("load service", "err", err)
		p.fail(ctx, log, deployID)
		return
	}
	envVars, err := p.store.ListEnvVars(ctx, d.ServiceID)
	if err != nil {
		log.Error("load env vars", "err", err)
		p.fail(ctx, log, deployID)
		return
	}

	// Custom domains only apply to production deploys, not per-PR previews.
	var domains []string
	if d.Kind == deploy.KindProduction {
		domains, err = p.domainNames(ctx, d.ServiceID)
		if err != nil {
			log.Error("load custom domains", "err", err)
			p.fail(ctx, log, deployID)
			return
		}
	}

	spec := k8s.ServiceSpec{
		Namespace: naming.NamespaceForOrg(orgID),
		Name:      naming.WorkloadName(svc.Slug, d.Kind, d.PRNumber),
		Type:      svc.Type,
		Image:     d.Image,
		Port:      servicePort,
		Schedule:  svc.Schedule,
		Env:       envMap(envVars),
		Domains:   domains,
	}
	if err := p.recon.Apply(ctx, spec); err != nil {
		log.Error("reconcile failed", "err", err)
		p.fail(ctx, log, deployID)
		return
	}

	if _, err := p.store.UpdateDeployStatus(ctx, deployID, deploy.StatusLive); err != nil {
		log.Error("transition to live", "err", err)
		return
	}
	log.Info("deploy live", "kind", d.Kind, "host", p.recon.Host(spec), "image", d.Image)
}

// teardown removes a preview environment. The payload is "<serviceID>:<pr>".
func (p *Processor) teardown(ctx context.Context, payload string) {
	serviceID, prNumber, err := parseTeardown(payload)
	if err != nil {
		p.log.Warn("bad teardown notification", "payload", payload, "err", err)
		return
	}
	log := p.log.With("service_id", serviceID, "pr", prNumber)

	svc, orgID, err := p.store.ServiceByID(ctx, serviceID)
	if err != nil {
		log.Error("load service for teardown", "err", err)
		return
	}
	namespace := naming.NamespaceForOrg(orgID)
	name := naming.WorkloadName(svc.Slug, deploy.KindPreview, &prNumber)
	if err := p.recon.Delete(ctx, namespace, name); err != nil {
		log.Error("teardown failed", "err", err)
		return
	}
	log.Info("preview torn down", "name", name)
}

// reconcileDatabase provisions a managed Postgres cluster, waits for it to be
// ready, and records the connection URI.
func (p *Processor) reconcileDatabase(ctx context.Context, dbID int64) {
	log := p.log.With("database_id", dbID)

	db, orgID, err := p.store.DatabaseByID(ctx, dbID)
	if err != nil {
		log.Error("load database", "err", err)
		return
	}
	namespace := naming.NamespaceForOrg(orgID)
	cluster := naming.DatabaseCluster(db.Slug)

	if err := p.recon.ProvisionPostgres(ctx, namespace, cluster); err != nil {
		log.Error("provision postgres", "err", err)
		p.failDatabase(ctx, log, dbID)
		return
	}
	if err := p.recon.WaitPostgresReady(ctx, namespace, cluster); err != nil {
		log.Error("wait postgres ready", "err", err)
		p.failDatabase(ctx, log, dbID)
		return
	}
	uri, err := p.recon.PostgresConnectionURI(ctx, namespace, cluster)
	if err != nil {
		log.Error("read connection uri", "err", err)
		p.failDatabase(ctx, log, dbID)
		return
	}
	if err := p.store.SetDatabaseConnection(ctx, dbID, uri); err != nil {
		log.Error("store connection uri", "err", err)
		return
	}
	log.Info("database ready", "cluster", cluster)
}

// teardownDatabase deletes a managed cluster. Payload is "<namespace>:<cluster>".
func (p *Processor) teardownDatabase(ctx context.Context, payload string) {
	namespace, cluster, ok := strings.Cut(payload, ":")
	if !ok {
		p.log.Warn("bad database teardown notification", "payload", payload)
		return
	}
	if err := p.recon.DeletePostgres(ctx, namespace, cluster); err != nil {
		p.log.Error("delete postgres", "cluster", cluster, "err", err)
		return
	}
	p.log.Info("database torn down", "cluster", cluster)
}

func (p *Processor) failDatabase(ctx context.Context, log *slog.Logger, dbID int64) {
	if err := p.store.SetDatabaseStatus(ctx, dbID, store.DBStatusFailed); err != nil {
		log.Error("mark database failed", "err", err)
	}
}

func (p *Processor) fail(ctx context.Context, log *slog.Logger, deployID int64) {
	if _, err := p.store.UpdateDeployStatus(ctx, deployID, deploy.StatusFailed); err != nil {
		log.Error("transition to failed", "err", err)
	}
}

// parseTeardown splits a "<serviceID>:<pr>" payload.
func parseTeardown(payload string) (serviceID int64, prNumber int, err error) {
	idStr, prStr, ok := strings.Cut(payload, ":")
	if !ok {
		return 0, 0, fmt.Errorf("expected <serviceID>:<pr>")
	}
	serviceID, err = strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	prNumber, err = strconv.Atoi(prStr)
	if err != nil {
		return 0, 0, err
	}
	return serviceID, prNumber, nil
}

// domainNames returns the custom domain hostnames attached to a service.
func (p *Processor) domainNames(ctx context.Context, serviceID int64) ([]string, error) {
	domains, err := p.store.ListCustomDomains(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(domains))
	for i, d := range domains {
		out[i] = d.Domain
	}
	return out, nil
}

// envMap flattens stored env vars into a key/value map for the reconciler.
func envMap(vars []store.EnvVar) map[string]string {
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		m[v.Key] = v.Value
	}
	return m
}
