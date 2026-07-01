package store

import (
	"context"
	"strconv"
	"strings"
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

// AuditQuery describes a filtered, paginated audit-log request.
type AuditQuery struct {
	Search string // case-insensitive substring match on path
	Method string // exact HTTP method filter (empty = any)
	Limit  int
	Offset int
}

// ListAudit returns a page of a user's audited actions matching the query,
// together with the total number of matches (for pagination).
func (s *Store) ListAudit(ctx context.Context, actorID int64, q AuditQuery) ([]AuditEntry, int, error) {
	// Build the shared WHERE clause + args dynamically.
	conds := []string{"actor_user_id = $1"}
	args := []any{actorID}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		conds = append(conds, "path ILIKE $"+strconv.Itoa(len(args)))
	}
	if q.Method != "" {
		args = append(args, q.Method)
		conds = append(conds, "method = $"+strconv.Itoa(len(args)))
	}
	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_logs "+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data page: append LIMIT/OFFSET args after the filter args.
	args = append(args, q.Limit, q.Offset)
	rows, err := s.pool.Query(ctx,
		"SELECT id, method, path, status, created_at FROM audit_logs "+where+
			" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Method, &e.Path, &e.Status, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
