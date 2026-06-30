// Package instance defines the named instance sizes a service can run at and
// their CPU/memory requests and limits. Shared by the API (validation) and the
// controller (resource construction) so they never drift.
package instance

// Size names. small–large are general-purpose; xlarge and 2xlarge are the
// "pro compute" tier for CPU/memory-heavy workloads.
const (
	Small   = "small"
	Medium  = "medium"
	Large   = "large"
	XLarge  = "xlarge"
	XXLarge = "2xlarge"
)

// Resources holds the Kubernetes resource quantities for a size.
type Resources struct {
	CPURequest string
	MemRequest string
	CPULimit   string
	MemLimit   string
}

var sizes = map[string]Resources{
	Small:   {CPURequest: "50m", MemRequest: "64Mi", CPULimit: "250m", MemLimit: "256Mi"},
	Medium:  {CPURequest: "250m", MemRequest: "256Mi", CPULimit: "1", MemLimit: "512Mi"},
	Large:   {CPURequest: "500m", MemRequest: "512Mi", CPULimit: "2", MemLimit: "1Gi"},
	XLarge:  {CPURequest: "1", MemRequest: "1Gi", CPULimit: "4", MemLimit: "4Gi"},
	XXLarge: {CPURequest: "2", MemRequest: "2Gi", CPULimit: "8", MemLimit: "8Gi"},
}

// IsValid reports whether s is a known instance size.
func IsValid(s string) bool {
	_, ok := sizes[s]
	return ok
}

// Get returns the resources for a size, falling back to Small for unknown input.
func Get(s string) Resources {
	if r, ok := sizes[s]; ok {
		return r
	}
	return sizes[Small]
}
