package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GithubConnection is an org's stored GitHub OAuth connection.
type GithubConnection struct {
	OrgID        int64
	AccessToken  string
	AccountLogin string
}

// SetGithubConnection upserts the org's GitHub OAuth token + account.
func (s *Store) SetGithubConnection(ctx context.Context, orgID int64, token, account string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO github_connections (org_id, access_token, account_login)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (org_id) DO UPDATE
		   SET access_token = EXCLUDED.access_token,
		       account_login = EXCLUDED.account_login`,
		orgID, token, account,
	)
	return err
}

// GithubConnection returns the org's connection, or (nil, nil) if none.
func (s *Store) GithubConnection(ctx context.Context, orgID int64) (*GithubConnection, error) {
	var c GithubConnection
	err := s.pool.QueryRow(ctx,
		`SELECT org_id, access_token, account_login FROM github_connections WHERE org_id = $1`,
		orgID,
	).Scan(&c.OrgID, &c.AccessToken, &c.AccountLogin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteGithubConnection removes the org's connection.
func (s *Store) DeleteGithubConnection(ctx context.Context, orgID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM github_connections WHERE org_id = $1`, orgID)
	return err
}
