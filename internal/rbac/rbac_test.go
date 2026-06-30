package rbac

import "testing"

func TestRanksAndRules(t *testing.T) {
	if !(Rank(Owner) > Rank(Admin) && Rank(Admin) > Rank(Member) && Rank(Member) > Rank(Viewer)) {
		t.Error("role ranks out of order")
	}
	if Rank("bogus") != 0 || IsValid("bogus") {
		t.Error("unknown role should rank 0 / be invalid")
	}
	if !AtLeast(Admin, Member) || AtLeast(Member, Admin) {
		t.Error("AtLeast comparison wrong")
	}
	if !CanWrite(Member) || !CanWrite(Owner) || CanWrite(Viewer) {
		t.Error("CanWrite should be member and above")
	}
}
