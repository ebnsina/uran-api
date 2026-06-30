package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// OrgMember is a user's membership in an org.
type OrgMember struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// RoleInOrg returns a user's role in an org, or ErrNotFound if not a member.
func (s *Store) RoleInOrg(ctx context.Context, userID, orgID int64) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM org_members WHERE org_id = $1 AND user_id = $2`,
		orgID, userID,
	).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

// ListOrgMembers returns the members of an org with their roles.
func (s *Store) ListOrgMembers(ctx context.Context, orgID int64) ([]OrgMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.id, u.email, u.name, m.role
		 FROM org_members m JOIN users u ON u.id = m.user_id
		 WHERE m.org_id = $1 ORDER BY u.id`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertOrgMember adds a user to an org with a role, or updates their role.
func (s *Store) UpsertOrgMember(ctx context.Context, orgID, userID int64, role string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO org_members (org_id, user_id, role) VALUES ($1, $2, $3)
		 ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		orgID, userID, role,
	)
	return err
}

// RemoveOrgMember removes a user from an org. Reports whether a row was deleted.
func (s *Store) RemoveOrgMember(ctx context.Context, orgID, userID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM org_members WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CountOrgOwners returns how many owners an org has (used to protect the last
// owner from removal/demotion).
func (s *Store) CountOrgOwners(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM org_members WHERE org_id = $1 AND role = 'owner'`, orgID,
	).Scan(&n)
	return n, err
}
