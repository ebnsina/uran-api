package store

import "context"

// EnvVar is a service environment variable.
type EnvVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// SetEnvVar upserts an environment variable for a service.
func (s *Store) SetEnvVar(ctx context.Context, serviceID int64, key, value string, secret bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO env_vars (service_id, key, value, secret)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (service_id, key)
		 DO UPDATE SET value = EXCLUDED.value, secret = EXCLUDED.secret`,
		serviceID, key, value, secret,
	)
	return err
}

// ListEnvVars returns a service's environment variables ordered by key.
func (s *Store) ListEnvVars(ctx context.Context, serviceID int64) ([]EnvVar, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, value, secret FROM env_vars WHERE service_id = $1 ORDER BY key`,
		serviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnvVar
	for rows.Next() {
		var e EnvVar
		if err := rows.Scan(&e.Key, &e.Value, &e.Secret); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteEnvVar removes an environment variable. It reports whether a row was
// deleted so the caller can return 404 for an unknown key.
func (s *Store) DeleteEnvVar(ctx context.Context, serviceID int64, key string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM env_vars WHERE service_id = $1 AND key = $2`,
		serviceID, key,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
