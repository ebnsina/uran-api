// Package rbac defines the org membership roles and their ordering. Higher-rank
// roles include the abilities of lower ones.
package rbac

// Roles, from lowest to highest privilege.
const (
	Viewer = "viewer" // read-only
	Member = "member" // deploy and manage services
	Admin  = "admin"  // manage members (member/viewer)
	Owner  = "owner"  // full control, incl. admins/owners and the org
)

var rank = map[string]int{Viewer: 1, Member: 2, Admin: 3, Owner: 4}

// IsValid reports whether r is a known role.
func IsValid(r string) bool { _, ok := rank[r]; return ok }

// Rank returns the privilege level of a role (0 for unknown).
func Rank(r string) int { return rank[r] }

// AtLeast reports whether role has at least min's privilege.
func AtLeast(role, min string) bool {
	return Rank(role) >= Rank(min) && Rank(role) > 0
}

// CanWrite reports whether a role may perform mutations (member and above).
func CanWrite(role string) bool { return AtLeast(role, Member) }
