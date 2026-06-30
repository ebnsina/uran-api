package store

import (
	"context"
	"time"
)

// MeteredService is the minimal info the sampler needs to read a service's
// metrics.
type MeteredService struct {
	ServiceID int64
	Slug      string
	OrgID     int64
}

// UsageSample is one resource-usage measurement.
type UsageSample struct {
	CPUMillicores int64     `json:"cpu_millicores"`
	MemoryBytes   int64     `json:"memory_bytes"`
	SampledAt     time.Time `json:"sampled_at"`
}

// ServicesForMetering lists non-suspended services with their org (for the
// metering sampler).
func (s *Store) ServicesForMetering(ctx context.Context) ([]MeteredService, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.slug, p.org_id
		 FROM services s JOIN projects p ON p.id = s.project_id
		 WHERE s.suspended = false`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MeteredService
	for rows.Next() {
		var m MeteredService
		if err := rows.Scan(&m.ServiceID, &m.Slug, &m.OrgID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// InsertUsageSample records a usage measurement for a service.
func (s *Store) InsertUsageSample(ctx context.Context, serviceID, cpuMilli, memBytes int64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO usage_samples (service_id, cpu_millicores, memory_bytes) VALUES ($1, $2, $3)`,
		serviceID, cpuMilli, memBytes,
	)
	return err
}

// ListUsageSamples returns a service's most recent usage samples.
func (s *Store) ListUsageSamples(ctx context.Context, serviceID int64, limit int) ([]UsageSample, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT cpu_millicores, memory_bytes, sampled_at FROM usage_samples
		 WHERE service_id = $1 ORDER BY id DESC LIMIT $2`,
		serviceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageSample
	for rows.Next() {
		var u UsageSample
		if err := rows.Scan(&u.CPUMillicores, &u.MemoryBytes, &u.SampledAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
