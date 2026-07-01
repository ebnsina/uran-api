package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CreateOrg inserts an org and adds the creator as owner, in one transaction.
func (s *Store) CreateOrg(ctx context.Context, userID int64, name, slug string) (Org, error) {
	var o Org
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return o, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO orgs (name, slug) VALUES ($1, $2)
		 RETURNING id, name, slug, created_at`,
		name, slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	if err != nil {
		return o, err
	}
	if _, err = tx.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
		o.ID, userID,
	); err != nil {
		return o, err
	}
	return o, tx.Commit(ctx)
}

// ListOrgs returns orgs the user is a member of.
func (s *Store) ListOrgs(ctx context.Context, userID int64) ([]Org, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT o.id, o.name, o.slug, o.created_at
		 FROM orgs o JOIN org_members m ON m.org_id = o.id
		 WHERE m.user_id = $1 ORDER BY o.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateOrg renames an org.
func (s *Store) UpdateOrg(ctx context.Context, id int64, name, slug string) (Org, error) {
	var o Org
	err := s.pool.QueryRow(ctx,
		`UPDATE orgs SET name = $2, slug = $3 WHERE id = $1
		 RETURNING id, name, slug, created_at`,
		id, name, slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt)
	return o, err
}

// DeleteOrg removes an org; projects, services and related rows cascade.
func (s *Store) DeleteOrg(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, id)
	return err
}

// IsOrgMember reports whether the user belongs to the org.
func (s *Store) IsOrgMember(ctx context.Context, userID, orgID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM org_members WHERE org_id = $1 AND user_id = $2)`,
		orgID, userID,
	).Scan(&exists)
	return exists, err
}

// CreateProject inserts a project under an org.
func (s *Store) CreateProject(ctx context.Context, orgID int64, name, slug string) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (org_id, name, slug) VALUES ($1, $2, $3)
		 RETURNING id, org_id, name, slug, created_at`,
		orgID, name, slug,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.CreatedAt)
	return p, err
}

// UpdateProject renames a project.
func (s *Store) UpdateProject(ctx context.Context, id int64, name, slug string) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`UPDATE projects SET name = $2, slug = $3 WHERE id = $1
		 RETURNING id, org_id, name, slug, created_at`,
		id, name, slug,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.CreatedAt)
	return p, err
}

// DeleteProject removes a project; its services and related rows cascade.
func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

// ProjectByID fetches a project. Returns ErrNotFound if absent.
func (s *Store) ProjectByID(ctx context.Context, id int64) (Project, error) {
	var p Project
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, slug, created_at FROM projects WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// ListProjects returns projects for an org.
func (s *Store) ListProjects(ctx context.Context, orgID int64) ([]Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, org_id, name, slug, created_at FROM projects WHERE org_id = $1 ORDER BY id`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// serviceCols is the canonical column list for selecting a service.
const serviceCols = `id, project_id, name, slug, type, repo_url, branch, schedule,
	replicas, instance_size, health_path, min_replicas, max_replicas, disk_size, disk_path, suspended, created_at`

func scanService(sc scanner) (Service, error) {
	var svc Service
	err := sc.Scan(&svc.ID, &svc.ProjectID, &svc.Name, &svc.Slug, &svc.Type, &svc.RepoURL, &svc.Branch, &svc.Schedule,
		&svc.Replicas, &svc.InstanceSize, &svc.HealthPath, &svc.MinReplicas, &svc.MaxReplicas, &svc.DiskSize, &svc.DiskPath, &svc.Suspended, &svc.CreatedAt)
	return svc, err
}

// CreateService inserts a service under a project.
func (s *Store) CreateService(ctx context.Context, projectID int64, name, slug, typ, repoURL, branch, schedule string) (Service, error) {
	return scanService(s.pool.QueryRow(ctx,
		`INSERT INTO services (project_id, name, slug, type, repo_url, branch, schedule)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+serviceCols,
		projectID, name, slug, typ, repoURL, branch, schedule,
	))
}

// ListServices returns services for a project.
func (s *Store) ListServices(ctx context.Context, projectID int64) ([]Service, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+serviceCols+` FROM services WHERE project_id = $1 ORDER BY id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}
