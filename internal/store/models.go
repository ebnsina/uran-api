package store

import "time"

type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

type Org struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type Project struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	ID           int64     `json:"id"`
	ProjectID    int64     `json:"project_id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Type         string    `json:"type"`
	RepoURL      string    `json:"repo_url"`
	Branch       string    `json:"branch"`
	Schedule     string    `json:"schedule,omitempty"`
	Replicas     int32     `json:"replicas"`
	InstanceSize string    `json:"instance_size"`
	HealthPath   string    `json:"health_path,omitempty"`
	MinReplicas  int32     `json:"min_replicas"`
	MaxReplicas  int32     `json:"max_replicas"`
	DiskSize     string    `json:"disk_size,omitempty"`
	DiskPath     string    `json:"disk_path,omitempty"`
	Suspended    bool      `json:"suspended"`
	CreatedAt    time.Time `json:"created_at"`
}

type Deploy struct {
	ID        int64     `json:"id"`
	ServiceID int64     `json:"service_id"`
	Status    string    `json:"status"`
	CommitSHA string    `json:"commit_sha"`
	Image     string    `json:"image"`
	Kind      string    `json:"kind"`
	PRNumber  *int      `json:"pr_number,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Build struct {
	ID        int64      `json:"id"`
	DeployID  int64      `json:"deploy_id"`
	Status    string     `json:"status"`
	Logs      string     `json:"logs"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
}
