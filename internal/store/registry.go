package store

import "context"

// RegistryCredential holds credentials for a private image registry. Password
// is omitted from JSON; it's only used internally to build the pull secret.
type RegistryCredential struct {
	ID       int64  `json:"id"`
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"-"`
}

// AddRegistryCredential upserts credentials for a registry within an org.
func (s *Store) AddRegistryCredential(ctx context.Context, orgID int64, registry, username, password string) (RegistryCredential, error) {
	var c RegistryCredential
	err := s.pool.QueryRow(ctx,
		`INSERT INTO registry_credentials (org_id, registry, username, password)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (org_id, registry)
		 DO UPDATE SET username = EXCLUDED.username, password = EXCLUDED.password
		 RETURNING id, registry, username, password`,
		orgID, registry, username, password,
	).Scan(&c.ID, &c.Registry, &c.Username, &c.Password)
	return c, err
}

// ListRegistryCredentials returns an org's registry credentials (with passwords,
// for building the pull secret — callers must not serialize the password).
func (s *Store) ListRegistryCredentials(ctx context.Context, orgID int64) ([]RegistryCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, registry, username, password FROM registry_credentials WHERE org_id = $1 ORDER BY id`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegistryCredential
	for rows.Next() {
		var c RegistryCredential
		if err := rows.Scan(&c.ID, &c.Registry, &c.Username, &c.Password); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteRegistryCredential removes a credential by id within an org.
func (s *Store) DeleteRegistryCredential(ctx context.Context, orgID, id int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM registry_credentials WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
