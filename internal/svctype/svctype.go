// Package svctype defines the kinds of services Uran can run and the rules that
// distinguish them (routable HTTP services vs background workers vs cron jobs).
package svctype

// Service types.
const (
	Web    = "web"    // long-running HTTP service, routed and TLS-terminated
	Static = "static" // built static site (currently served like a web service)
	Worker = "worker" // long-running background process, no inbound routing
	Cron   = "cron"   // scheduled job
)

// IsValid reports whether t is a known service type.
func IsValid(t string) bool {
	switch t {
	case Web, Static, Worker, Cron:
		return true
	default:
		return false
	}
}

// RequiresSchedule reports whether the type needs a cron schedule.
func RequiresSchedule(t string) bool { return t == Cron }

// IsRoutable reports whether the type is exposed over HTTP (Service + route).
func IsRoutable(t string) bool { return t == Web || t == Static }
