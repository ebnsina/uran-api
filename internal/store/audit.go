package store

import (
	"context"
	"time"
)

// AuditEntry is one recorded action.
type AuditEntry struct {
	ID        int64     `json:"id"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// InsertAudit records an action. It is best-effort: errors are ignored so
// auditing never breaks the request path.
func (s *Store) InsertAudit(ctx context.Context, actorID int64, actorEmail, method, path string, status int) {
	_, _ = s.pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_user_id, actor_email, method, path, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorID, actorEmail, method, path, status,
	)
}

// ListAudit returns a user's most recent audited actions.
func (s *Store) ListAudit(ctx context.Context, actorID int64, limit int) ([]AuditEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, method, path, status, created_at FROM audit_logs
		 WHERE actor_user_id = $1 ORDER BY id DESC LIMIT $2`,
		actorID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Method, &e.Path, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
