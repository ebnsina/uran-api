package controller

import "testing"

func TestParseTeardown(t *testing.T) {
	id, pr, err := parseTeardown("7:42")
	if err != nil || id != 7 || pr != 42 {
		t.Fatalf("parseTeardown(7:42) = (%d, %d, %v)", id, pr, err)
	}

	for _, bad := range []string{"", "7", "7:", "x:1", "7:y", "7:1:2"} {
		if _, _, err := parseTeardown(bad); err == nil {
			t.Errorf("parseTeardown(%q) expected error", bad)
		}
	}
}
