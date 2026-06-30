package instance

import "testing"

func TestSizes(t *testing.T) {
	if !IsValid(Small) || !IsValid(Medium) || !IsValid(Large) || IsValid("huge") {
		t.Error("IsValid mismatch")
	}
	// Unknown size falls back to small.
	if Get("huge") != Get(Small) {
		t.Error("unknown size should fall back to small")
	}
	if Get(Large).CPULimit != "2" {
		t.Errorf("large CPU limit = %q, want 2", Get(Large).CPULimit)
	}
}
