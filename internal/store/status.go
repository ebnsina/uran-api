package store

import "context"

// ServiceStatus summarizes a service for the status page.
type ServiceStatus struct {
	ServiceID int64  `json:"service_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Suspended bool   `json:"suspended"`
	Status    string `json:"status"` // latest deploy status, or "none"
}

// ProjectServiceStatuses returns each service in a project with its latest
// deploy status.
func (s *Store) ProjectServiceStatuses(ctx context.Context, projectID int64) ([]ServiceStatus, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.name, s.type, s.suspended,
		        coalesce((SELECT d.status FROM deploys d
		                  WHERE d.service_id = s.id ORDER BY d.id DESC LIMIT 1), 'none')
		 FROM services s WHERE s.project_id = $1 ORDER BY s.id`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceStatus
	for rows.Next() {
		var st ServiceStatus
		if err := rows.Scan(&st.ServiceID, &st.Name, &st.Type, &st.Suspended, &st.Status); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
