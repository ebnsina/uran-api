package naming

import (
	"testing"

	"github.com/ebnsina/uran-api/internal/deploy"
)

func TestNamespaceForOrg(t *testing.T) {
	if got := NamespaceForOrg(42); got != "uran-org-42" {
		t.Errorf("NamespaceForOrg(42) = %q", got)
	}
}

func TestWorkloadName(t *testing.T) {
	if got := WorkloadName("web", deploy.KindProduction, nil); got != "web" {
		t.Errorf("production = %q, want web", got)
	}
	pr := 7
	if got := WorkloadName("web", deploy.KindPreview, &pr); got != "web-pr-7" {
		t.Errorf("preview = %q, want web-pr-7", got)
	}
	// Preview without a PR number falls back to the base slug.
	if got := WorkloadName("web", deploy.KindPreview, nil); got != "web" {
		t.Errorf("preview without pr = %q, want web", got)
	}
}

func TestDatabaseCluster(t *testing.T) {
	if got := DatabaseCluster("maindb"); got != "db-maindb" {
		t.Errorf("DatabaseCluster = %q, want db-maindb", got)
	}
}
