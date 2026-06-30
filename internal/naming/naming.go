// Package naming centralizes how Uran derives Kubernetes object names from
// domain objects, so the API and controller stay in agreement.
package naming

import (
	"fmt"

	"github.com/ebnsina/uran-api/internal/deploy"
)

// NamespaceForOrg returns the namespace that isolates an org's services.
func NamespaceForOrg(orgID int64) string {
	return fmt.Sprintf("uran-org-%d", orgID)
}

// WorkloadName returns the Deployment/Service/IngressRoute name for a deploy.
// Production deploys use the service slug; preview deploys are suffixed with the
// pull request number so they live alongside production without colliding.
func WorkloadName(slug, kind string, prNumber *int) string {
	if kind == deploy.KindPreview && prNumber != nil {
		return fmt.Sprintf("%s-pr-%d", slug, *prNumber)
	}
	return slug
}

// DatabaseCluster returns the managed-database cluster name for a db slug. The
// "db-" prefix keeps it from colliding with service workloads in the namespace.
func DatabaseCluster(slug string) string {
	return "db-" + slug
}
