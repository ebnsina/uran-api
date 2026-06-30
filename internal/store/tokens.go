package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// APIToken is a personal access token's metadata (never the secret itself).
type APIToken struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// CreateAPIToken stores a token's hash and display prefix for a user.
func (s *Store) CreateAPIToken(ctx context.Context, userID int64, name, tokenHash, prefix string) (APIToken, error) {
	var t APIToken
	err := s.pool.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, token_hash, prefix) VALUES ($1, $2, $3, $4)
		 RETURNING id, name, prefix, created_at, last_used_at`,
		userID, name, tokenHash, prefix,
	).Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt)
	return t, err
}

// ListAPITokens returns a user's tokens (metadata only).
func (s *Store) ListAPITokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, prefix, created_at, last_used_at FROM api_tokens WHERE user_id = $1 ORDER BY id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteAPIToken removes a user's token. Reports whether a row was deleted.
func (s *Store) DeleteAPIToken(ctx context.Context, userID, id int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UserByAPIToken resolves a token hash to its owning user. Returns ErrNotFound
// if no token matches.
func (s *Store) UserByAPIToken(ctx context.Context, tokenHash string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.password_hash, u.name, u.created_at
		 FROM api_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = $1`,
		tokenHash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// TouchAPIToken records that a token was just used (best effort).
func (s *Store) TouchAPIToken(ctx context.Context, tokenHash string) {
	_, _ = s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE token_hash = $1`, tokenHash)
}
