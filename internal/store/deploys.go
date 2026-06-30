package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ebnsina/uran-api/internal/deploy"
)

// ErrInvalidTransition is returned when a deploy status update is not allowed
// by the state machine.
var ErrInvalidTransition = errors.New("invalid status transition")

// deployCols is the canonical column list for selecting a deploy.
const deployCols = `id, service_id, status, commit_sha, image, kind, pr_number, created_at, updated_at`

// scanner is satisfied by both pgx.Row and pgx.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanDeploy(s scanner) (Deploy, error) {
	var d Deploy
	err := s.Scan(&d.ID, &d.ServiceID, &d.Status, &d.CommitSHA, &d.Image, &d.Kind, &d.PRNumber, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// CreateDeploy inserts a queued production deploy with its queued build.
func (s *Store) CreateDeploy(ctx context.Context, serviceID int64, commitSHA string) (Deploy, error) {
	return s.createBuildableDeploy(ctx, serviceID, commitSHA, deploy.KindProduction, nil)
}

// CreatePreviewDeploy inserts a queued preview deploy for a pull request.
func (s *Store) CreatePreviewDeploy(ctx context.Context, serviceID int64, commitSHA string, prNumber int) (Deploy, error) {
	return s.createBuildableDeploy(ctx, serviceID, commitSHA, deploy.KindPreview, &prNumber)
}

// createBuildableDeploy inserts a queued deploy (+ queued build) of the given
// kind, in one transaction.
func (s *Store) createBuildableDeploy(ctx context.Context, serviceID int64, commitSHA, kind string, prNumber *int) (Deploy, error) {
	var d Deploy
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)

	d, err = scanDeploy(tx.QueryRow(ctx,
		`INSERT INTO deploys (service_id, status, commit_sha, kind, pr_number)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+deployCols,
		serviceID, deploy.StatusQueued, commitSHA, kind, prNumber,
	))
	if err != nil {
		return d, err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO builds (deploy_id, status) VALUES ($1, $2)`,
		d.ID, deploy.BuildQueued,
	); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}

// CreateRollbackDeploy creates a new deploy that reuses an already-built image,
// skipping the build stage by inserting it directly in the "deploying" state
// (with a synthetic succeeded build for log/history symmetry). The controller
// picks it up and reconciles it onto the cluster.
func (s *Store) CreateRollbackDeploy(ctx context.Context, serviceID int64, image, commitSHA string, fromDeployID int64) (Deploy, error) {
	var d Deploy
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)

	d, err = scanDeploy(tx.QueryRow(ctx,
		`INSERT INTO deploys (service_id, status, commit_sha, image, kind)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+deployCols,
		serviceID, deploy.StatusDeploying, commitSHA, image, deploy.KindProduction,
	))
	if err != nil {
		return d, err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO builds (deploy_id, status, logs, started_at, ended_at)
		 VALUES ($1, $2, $3, now(), now())`,
		d.ID, deploy.BuildSucceeded,
		fmt.Sprintf("rolled back: reused image %s from deploy %d\n", image, fromDeployID),
	); err != nil {
		return d, err
	}
	return d, tx.Commit(ctx)
}

// DeployByID fetches a single deploy.
func (s *Store) DeployByID(ctx context.Context, id int64) (Deploy, error) {
	d, err := scanDeploy(s.pool.QueryRow(ctx,
		`SELECT `+deployCols+` FROM deploys WHERE id = $1`, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// ListDeploys returns deploys for a service, newest first.
func (s *Store) ListDeploys(ctx context.Context, serviceID int64) ([]Deploy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+deployCols+` FROM deploys WHERE service_id = $1 ORDER BY id DESC`,
		serviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deploy
	for rows.Next() {
		d, err := scanDeploy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDeployStatus moves a deploy to a new status, enforcing the state
// machine. Returns ErrInvalidTransition if the move is not allowed.
func (s *Store) UpdateDeployStatus(ctx context.Context, id int64, to string) (Deploy, error) {
	current, err := s.DeployByID(ctx, id)
	if err != nil {
		return Deploy{}, err
	}
	if !deploy.CanTransition(current.Status, to) {
		return Deploy{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, to)
	}
	return scanDeploy(s.pool.QueryRow(ctx,
		`UPDATE deploys SET status = $2, updated_at = now() WHERE id = $1 RETURNING `+deployCols,
		id, to,
	))
}

// SetDeployImage records the built image reference for a deploy.
func (s *Store) SetDeployImage(ctx context.Context, id int64, image string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE deploys SET image = $2, updated_at = now() WHERE id = $1`,
		id, image,
	)
	return err
}

// ServicesByRepo returns services whose repo URL and branch match a push.
// Matching is case-insensitive on the repo URL and tolerant of a trailing
// ".git" suffix so both clone and html URLs resolve.
func (s *Store) ServicesByRepo(ctx context.Context, repoURL, branch string) ([]Service, error) {
	return s.queryServices(ctx,
		`SELECT id, project_id, name, slug, type, repo_url, branch, created_at
		 FROM services
		 WHERE branch = $2
		   AND lower(trim(trailing '.git' from repo_url)) = lower(trim(trailing '.git' from $1))`,
		repoURL, branch,
	)
}

// ServicesByRepoURL returns all services for a repo regardless of branch, used
// to fan a pull request out to every service of that repo.
func (s *Store) ServicesByRepoURL(ctx context.Context, repoURL string) ([]Service, error) {
	return s.queryServices(ctx,
		`SELECT id, project_id, name, slug, type, repo_url, branch, created_at
		 FROM services
		 WHERE lower(trim(trailing '.git' from repo_url)) = lower(trim(trailing '.git' from $1))`,
		repoURL,
	)
}

func (s *Store) queryServices(ctx context.Context, sql string, args ...any) ([]Service, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Slug, &svc.Type, &svc.RepoURL, &svc.Branch, &svc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

// ServiceByID fetches a service together with the org it belongs to (for access
// checks). Returns ErrNotFound if absent.
func (s *Store) ServiceByID(ctx context.Context, id int64) (Service, int64, error) {
	var svc Service
	var orgID int64
	err := s.pool.QueryRow(ctx,
		`SELECT s.id, s.project_id, s.name, s.slug, s.type, s.repo_url, s.branch, s.created_at, p.org_id
		 FROM services s JOIN projects p ON p.id = s.project_id
		 WHERE s.id = $1`,
		id,
	).Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Slug, &svc.Type, &svc.RepoURL, &svc.Branch, &svc.CreatedAt, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return svc, 0, ErrNotFound
	}
	return svc, orgID, err
}
