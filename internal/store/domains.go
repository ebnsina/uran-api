package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDuplicate indicates a unique-constraint violation (e.g. a domain already
// claimed by some service).
var ErrDuplicate = errors.New("already exists")

// CustomDomain is a user-supplied hostname routed to a service.
type CustomDomain struct {
	ID        int64  `json:"id"`
	ServiceID int64  `json:"service_id"`
	Domain    string `json:"domain"`
}

// AddCustomDomain attaches a domain to a service. Returns ErrDuplicate if the
// domain is already claimed.
func (s *Store) AddCustomDomain(ctx context.Context, serviceID int64, domain string) (CustomDomain, error) {
	var d CustomDomain
	err := s.pool.QueryRow(ctx,
		`INSERT INTO custom_domains (service_id, domain) VALUES ($1, $2)
		 RETURNING id, service_id, domain`,
		serviceID, domain,
	).Scan(&d.ID, &d.ServiceID, &d.Domain)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return d, ErrDuplicate
	}
	return d, err
}

// ListCustomDomains returns the domains attached to a service.
func (s *Store) ListCustomDomains(ctx context.Context, serviceID int64) ([]CustomDomain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, service_id, domain FROM custom_domains WHERE service_id = $1 ORDER BY domain`,
		serviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomDomain
	for rows.Next() {
		var d CustomDomain
		if err := rows.Scan(&d.ID, &d.ServiceID, &d.Domain); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteCustomDomain removes a domain from a service. Reports whether a row was
// deleted.
func (s *Store) DeleteCustomDomain(ctx context.Context, serviceID int64, domain string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM custom_domains WHERE service_id = $1 AND domain = $2`,
		serviceID, domain,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
