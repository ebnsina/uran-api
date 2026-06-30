package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ebnsina/uran-api/internal/instance"
	"github.com/ebnsina/uran-api/internal/k8s"
	"github.com/ebnsina/uran-api/internal/naming"
	"github.com/ebnsina/uran-api/internal/store"
)

const (
	dbAutoscaleInterval = 20 * time.Second
	dbScaleUpPct        = 70 // scale up above this CPU utilization (% of request)
	dbScaleDownPct      = 30 // scale down below this
	dbAutoscaleCooldown = 60 * time.Second
)

// runDBAutoscaler periodically scales autoscale-tier Postgres databases between
// their min and max instances based on CPU utilization.
func (p *Processor) runDBAutoscaler(ctx context.Context) error {
	p.log.Info("database autoscaler started", "interval", dbAutoscaleInterval.String())
	ticker := time.NewTicker(dbAutoscaleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.autoscaleDatabases(ctx)
		}
	}
}

func (p *Processor) autoscaleDatabases(ctx context.Context) {
	dbs, err := p.store.ListAutoscaleDatabases(ctx)
	if err != nil {
		p.log.Warn("autoscaler: list databases", "err", err)
		return
	}
	for _, db := range dbs {
		p.autoscaleOne(ctx, db)
	}
}

func (p *Processor) autoscaleOne(ctx context.Context, db store.AutoscaleDatabase) {
	namespace := naming.NamespaceForOrg(db.OrgID)
	cluster := naming.DatabaseCluster(db.Slug)

	util, ok := p.clusterCPUUtilization(ctx, namespace, cluster, db.Size)
	if !ok {
		return // metrics not available yet
	}

	desired := db.Instances
	switch {
	case util > dbScaleUpPct && db.Instances < db.MaxInstances:
		desired = db.Instances + 1
	case util < dbScaleDownPct && db.Instances > db.MinInstances:
		desired = db.Instances - 1
	}
	if desired == db.Instances || !p.cooldownElapsed(db.ID) {
		return
	}

	log := p.log.With("database_id", db.ID, "cluster", cluster)
	if err := p.recon.ProvisionPostgres(ctx, namespace, cluster, k8s.PostgresSpec{
		Instances: desired, Size: db.Size, Storage: db.Storage,
	}); err != nil {
		log.Error("autoscaler: provision", "err", err)
		return
	}
	if err := p.store.SetDatabaseInstances(ctx, db.ID, desired); err != nil {
		log.Error("autoscaler: persist instances", "err", err)
		return
	}
	p.markScaled(db.ID)
	log.Info("autoscaled database", "from", db.Instances, "to", desired, "cpu_util_pct", util)
}

// clusterCPUUtilization returns average CPU utilization (% of per-pod request)
// across a cluster's pods.
func (p *Processor) clusterCPUUtilization(ctx context.Context, namespace, cluster, size string) (int64, bool) {
	metrics, err := p.reader.Metrics(ctx, namespace, "cnpg.io/cluster="+cluster)
	if err != nil || len(metrics) == 0 {
		return 0, false
	}
	req := resource.MustParse(instance.Get(size).CPURequest)
	requestMilli := req.MilliValue()
	if requestMilli == 0 {
		return 0, false
	}
	var total int64
	for _, m := range metrics {
		total += m.CPUMillicores
	}
	avg := total / int64(len(metrics))
	return avg * 100 / requestMilli, true
}

func (p *Processor) cooldownElapsed(dbID int64) bool {
	p.scaleMu.Lock()
	defer p.scaleMu.Unlock()
	last, ok := p.lastScale[dbID]
	return !ok || time.Since(last) >= dbAutoscaleCooldown
}

func (p *Processor) markScaled(dbID int64) {
	p.scaleMu.Lock()
	defer p.scaleMu.Unlock()
	p.lastScale[dbID] = time.Now()
}
