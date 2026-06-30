package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// BuildByDeployID returns the build belonging to a deploy.
func (s *Store) BuildByDeployID(ctx context.Context, deployID int64) (Build, error) {
	var b Build
	err := s.pool.QueryRow(ctx,
		`SELECT id, deploy_id, status, logs, started_at, ended_at
		 FROM builds WHERE deploy_id = $1`,
		deployID,
	).Scan(&b.ID, &b.DeployID, &b.Status, &b.Logs, &b.StartedAt, &b.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

// StartBuild marks a build as running and stamps started_at.
func (s *Store) StartBuild(ctx context.Context, buildID int64, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE builds SET status = $2, started_at = now() WHERE id = $1`,
		buildID, status,
	)
	return err
}

// FinishBuild sets a terminal build status and stamps ended_at.
func (s *Store) FinishBuild(ctx context.Context, buildID int64, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE builds SET status = $2, ended_at = now() WHERE id = $1`,
		buildID, status,
	)
	return err
}

// AppendBuildLog appends a chunk to a build's log column.
func (s *Store) AppendBuildLog(ctx context.Context, buildID int64, chunk string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE builds SET logs = logs || $2 WHERE id = $1`,
		buildID, chunk,
	)
	return err
}
