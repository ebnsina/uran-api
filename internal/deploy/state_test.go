package deploy

import "testing"

func TestCanTransition(t *testing.T) {
	valid := [][2]string{
		{StatusQueued, StatusBuilding},
		{StatusQueued, StatusFailed},
		{StatusBuilding, StatusDeploying},
		{StatusBuilding, StatusFailed},
		{StatusDeploying, StatusLive},
		{StatusDeploying, StatusFailed},
	}
	for _, tc := range valid {
		if !CanTransition(tc[0], tc[1]) {
			t.Errorf("expected %s -> %s to be allowed", tc[0], tc[1])
		}
	}

	invalid := [][2]string{
		{StatusQueued, StatusLive},     // can't skip build/deploy
		{StatusLive, StatusBuilding},   // terminal
		{StatusFailed, StatusQueued},   // terminal
		{StatusDeploying, StatusQueued}, // no going back
	}
	for _, tc := range invalid {
		if CanTransition(tc[0], tc[1]) {
			t.Errorf("expected %s -> %s to be disallowed", tc[0], tc[1])
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(StatusLive) || !IsTerminal(StatusFailed) {
		t.Error("live and failed should be terminal")
	}
	if IsTerminal(StatusQueued) || IsTerminal(StatusBuilding) {
		t.Error("queued and building should not be terminal")
	}
}
