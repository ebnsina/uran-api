package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SetServiceScaling updates a service's replica count, instance size, and
// autoscaling bounds.
func (s *Store) SetServiceScaling(ctx context.Context, id int64, replicas int32, instanceSize string, minReplicas, maxReplicas int32) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE services SET replicas = $2, instance_size = $3, min_replicas = $4, max_replicas = $5 WHERE id = $1`,
		id, replicas, instanceSize, minReplicas, maxReplicas,
	)
	return err
}

// SetServiceHealth updates a service's health-check path.
func (s *Store) SetServiceHealth(ctx context.Context, id int64, path string) error {
	_, err := s.pool.Exec(ctx, `UPDATE services SET health_path = $2 WHERE id = $1`, id, path)
	return err
}

// LatestImagedDeploy returns the newest deploy for a service that has a built
// image (so settings changes can be re-applied without a rebuild). Returns
// ErrNotFound if the service has never produced an image.
func (s *Store) LatestImagedDeploy(ctx context.Context, serviceID int64) (Deploy, error) {
	d, err := scanDeploy(s.pool.QueryRow(ctx,
		`SELECT `+deployCols+` FROM deploys
		 WHERE service_id = $1 AND image <> '' ORDER BY id DESC LIMIT 1`,
		serviceID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}
