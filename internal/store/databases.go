package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Database statuses.
const (
	DBStatusCreating = "creating"
	DBStatusReady    = "ready"
	DBStatusFailed   = "failed"
)

// Database is a managed database provisioned for a project. ConnectionURI holds
// credentials and is never serialized in list/get responses — it is returned
// only by the dedicated connection endpoint.
type Database struct {
	ID            int64  `json:"id"`
	ProjectID     int64  `json:"project_id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Engine        string `json:"engine"`
	Status        string `json:"status"`
	Tier          string `json:"tier"`
	Instances     int32  `json:"instances"`
	MinInstances  int32  `json:"min_instances"`
	MaxInstances  int32  `json:"max_instances"`
	Size          string `json:"size"`
	Storage       string `json:"storage"`
	ConnectionURI string `json:"-"`
	ReadURI       string `json:"-"`
}

const databaseCols = `id, project_id, name, slug, engine, status, tier, instances, min_instances, max_instances, size, storage, connection_uri, read_uri`

func scanDatabase(sc scanner) (Database, error) {
	var d Database
	err := sc.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Slug, &d.Engine, &d.Status, &d.Tier,
		&d.Instances, &d.MinInstances, &d.MaxInstances, &d.Size, &d.Storage, &d.ConnectionURI, &d.ReadURI)
	return d, err
}

// CreateDatabase inserts a database record in the "creating" state.
func (s *Store) CreateDatabase(ctx context.Context, projectID int64, name, slug, engine, tier string, instances, minInstances, maxInstances int32, size, storage string) (Database, error) {
	return scanDatabase(s.pool.QueryRow(ctx,
		`INSERT INTO databases (project_id, name, slug, engine, tier, instances, min_instances, max_instances, size, storage)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING `+databaseCols,
		projectID, name, slug, engine, tier, instances, minInstances, maxInstances, size, storage,
	))
}

// AutoscaleDatabase is a ready autoscale database plus the org it belongs to.
type AutoscaleDatabase struct {
	Database
	OrgID int64
}

// ListAutoscaleDatabases returns ready autoscale-tier databases for the
// controller's autoscaler loop.
func (s *Store) ListAutoscaleDatabases(ctx context.Context) ([]AutoscaleDatabase, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.id, d.project_id, d.name, d.slug, d.engine, d.status, d.tier, d.instances,
		        d.min_instances, d.max_instances, d.size, d.storage, d.connection_uri, d.read_uri, p.org_id
		 FROM databases d JOIN projects p ON p.id = d.project_id
		 WHERE d.tier = 'autoscale' AND d.status = $1`,
		DBStatusReady,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutoscaleDatabase
	for rows.Next() {
		var a AutoscaleDatabase
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Slug, &a.Engine, &a.Status, &a.Tier, &a.Instances,
			&a.MinInstances, &a.MaxInstances, &a.Size, &a.Storage, &a.ConnectionURI, &a.ReadURI, &a.OrgID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetDatabaseInstances updates only the current instance count (used by the
// autoscaler; does not change status).
func (s *Store) SetDatabaseInstances(ctx context.Context, id int64, instances int32) error {
	_, err := s.pool.Exec(ctx, `UPDATE databases SET instances = $2 WHERE id = $1`, id, instances)
	return err
}

// DatabaseByID fetches a database with its owning org id (for access checks).
func (s *Store) DatabaseByID(ctx context.Context, id int64) (Database, int64, error) {
	var d Database
	var orgID int64
	err := s.pool.QueryRow(ctx,
		`SELECT d.id, d.project_id, d.name, d.slug, d.engine, d.status, d.tier,
		        d.instances, d.min_instances, d.max_instances, d.size, d.storage, d.connection_uri, d.read_uri, p.org_id
		 FROM databases d JOIN projects p ON p.id = d.project_id
		 WHERE d.id = $1`,
		id,
	).Scan(&d.ID, &d.ProjectID, &d.Name, &d.Slug, &d.Engine, &d.Status, &d.Tier,
		&d.Instances, &d.MinInstances, &d.MaxInstances, &d.Size, &d.Storage, &d.ConnectionURI, &d.ReadURI, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, 0, ErrNotFound
	}
	return d, orgID, err
}

// SetDatabaseScaling updates instances/size/storage and marks the database
// updating so the controller re-reconciles.
func (s *Store) SetDatabaseScaling(ctx context.Context, id int64, instances int32, size, storage string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE databases SET instances = $2, size = $3, storage = $4, status = $5 WHERE id = $1`,
		id, instances, size, storage, DBStatusCreating,
	)
	return err
}

// ListDatabases returns databases for a project.
func (s *Store) ListDatabases(ctx context.Context, projectID int64) ([]Database, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+databaseCols+` FROM databases WHERE project_id = $1 ORDER BY id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Database
	for rows.Next() {
		d, err := scanDatabase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDatabaseStatus updates a database's status.
func (s *Store) SetDatabaseStatus(ctx context.Context, id int64, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE databases SET status = $2 WHERE id = $1`, id, status)
	return err
}

// SetDatabaseConnection stores the connection URIs and marks the database ready.
func (s *Store) SetDatabaseConnection(ctx context.Context, id int64, uri, readURI string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE databases SET connection_uri = $2, read_uri = $3, status = $4 WHERE id = $1`,
		id, uri, readURI, DBStatusReady,
	)
	return err
}

// DeleteDatabase removes a database record.
func (s *Store) DeleteDatabase(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM databases WHERE id = $1`, id)
	return err
}
