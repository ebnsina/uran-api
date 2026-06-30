package svctype

import "testing"

func TestRules(t *testing.T) {
	if !IsValid(Web) || !IsValid(Cron) || IsValid("bogus") {
		t.Error("IsValid mismatch")
	}
	if !RequiresSchedule(Cron) || RequiresSchedule(Web) {
		t.Error("RequiresSchedule mismatch")
	}
	if !IsRoutable(Web) || !IsRoutable(Static) {
		t.Error("web/static should be routable")
	}
	if IsRoutable(Worker) || IsRoutable(Cron) {
		t.Error("worker/cron should not be routable")
	}
}
