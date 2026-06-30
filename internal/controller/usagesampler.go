package controller

import (
	"context"
	"time"

	"github.com/ebnsina/uran-api/internal/naming"
)

// usageSampleInterval is how often service resource usage is recorded.
const usageSampleInterval = 60 * time.Second

// runUsageSampler periodically records each running service's CPU/memory usage
// for metering.
func (p *Processor) runUsageSampler(ctx context.Context) error {
	p.log.Info("usage sampler started", "interval", usageSampleInterval.String())
	ticker := time.NewTicker(usageSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.sampleUsage(ctx)
		}
	}
}

func (p *Processor) sampleUsage(ctx context.Context) {
	services, err := p.store.ServicesForMetering(ctx)
	if err != nil {
		p.log.Warn("usage sampler: list services", "err", err)
		return
	}
	for _, svc := range services {
		namespace := naming.NamespaceForOrg(svc.OrgID)
		metrics, err := p.reader.Metrics(ctx, namespace, "app.kubernetes.io/name="+svc.Slug)
		if err != nil || len(metrics) == 0 {
			continue // not running / no metrics
		}
		var cpu, mem int64
		for _, m := range metrics {
			cpu += m.CPUMillicores
			mem += m.MemoryBytes
		}
		if err := p.store.InsertUsageSample(ctx, svc.ServiceID, cpu, mem); err != nil {
			p.log.Warn("usage sampler: insert", "service_id", svc.ServiceID, "err", err)
		}
	}
}
