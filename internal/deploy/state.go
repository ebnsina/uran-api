// Package deploy defines the deploy lifecycle and its valid state transitions.
package deploy

// Status values for a deploy.
const (
	StatusQueued    = "queued"
	StatusBuilding  = "building"
	StatusDeploying = "deploying"
	StatusLive      = "live"
	StatusFailed    = "failed"
)

// Build status values.
const (
	BuildQueued    = "queued"
	BuildRunning   = "running"
	BuildSucceeded = "succeeded"
	BuildFailed    = "failed"
)

// Deploy kinds distinguish a service's production deploys from ephemeral
// per-pull-request preview deploys.
const (
	KindProduction = "production"
	KindPreview    = "preview"
)

// transitions lists the allowed next states for each deploy status.
var transitions = map[string][]string{
	StatusQueued:    {StatusBuilding, StatusFailed},
	StatusBuilding:  {StatusDeploying, StatusFailed},
	StatusDeploying: {StatusLive, StatusFailed},
	StatusLive:      {}, // terminal (a new deploy supersedes it)
	StatusFailed:    {}, // terminal
}

// CanTransition reports whether a deploy may move from->to.
func CanTransition(from, to string) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether a status has no further transitions.
func IsTerminal(status string) bool {
	return len(transitions[status]) == 0
}
