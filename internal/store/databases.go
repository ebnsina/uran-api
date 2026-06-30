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
	ConnectionURI string `json:"-"`
}

const databaseCols = `id, project_id, name, slug, engine, status, connection_uri`

func scanDatabase(sc scanner) (Database, error) {
	var d Database
	err := sc.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Slug, &d.Engine, &d.Status, &d.ConnectionURI)
	return d, err
}

// CreateDatabase inserts a database record in the "creating" state.
func (s *Store) CreateDatabase(ctx context.Context, projectID int64, name, slug, engine string) (Database, error) {
	return scanDatabase(s.pool.QueryRow(ctx,
		`INSERT INTO databases (project_id, name, slug, engine) VALUES ($1, $2, $3, $4)
		 RETURNING `+databaseCols,
		projectID, name, slug, engine,
	))
}

// DatabaseByID fetches a database with its owning org id (for access checks).
func (s *Store) DatabaseByID(ctx context.Context, id int64) (Database, int64, error) {
	var d Database
	var orgID int64
	err := s.pool.QueryRow(ctx,
		`SELECT d.id, d.project_id, d.name, d.slug, d.engine, d.status, d.connection_uri, p.org_id
		 FROM databases d JOIN projects p ON p.id = d.project_id
		 WHERE d.id = $1`,
		id,
	).Scan(&d.ID, &d.ProjectID, &d.Name, &d.Slug, &d.Engine, &d.Status, &d.ConnectionURI, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, 0, ErrNotFound
	}
	return d, orgID, err
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

// SetDatabaseConnection stores the connection URI and marks the database ready.
func (s *Store) SetDatabaseConnection(ctx context.Context, id int64, uri string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE databases SET connection_uri = $2, status = $3 WHERE id = $1`,
		id, uri, DBStatusReady,
	)
	return err
}

// DeleteDatabase removes a database record.
func (s *Store) DeleteDatabase(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM databases WHERE id = $1`, id)
	return err
}
