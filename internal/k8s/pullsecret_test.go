package k8s

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDockerConfigJSON(t *testing.T) {
	data, err := dockerConfigJSON([]RegistryCred{
		{Registry: "ghcr.io", Username: "bot", Password: "tok"},
	})
	if err != nil {
		t.Fatalf("dockerConfigJSON: %v", err)
	}
	var parsed struct {
		Auths map[string]struct {
			Username, Password, Auth string
		}
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	e, ok := parsed.Auths["ghcr.io"]
	if !ok {
		t.Fatal("missing ghcr.io entry")
	}
	if e.Username != "bot" || e.Password != "tok" {
		t.Errorf("creds = %+v", e)
	}
	want := base64.StdEncoding.EncodeToString([]byte("bot:tok"))
	if e.Auth != want {
		t.Errorf("auth = %q, want %q", e.Auth, want)
	}
}
