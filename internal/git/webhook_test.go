package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	secret := "topsecret"
	good := sign(secret, body)

	if !VerifySignature(secret, body, good) {
		t.Error("valid signature should verify")
	}
	if VerifySignature(secret, body, "sha256=deadbeef") {
		t.Error("invalid signature should not verify")
	}
	if VerifySignature(secret, body, "") {
		t.Error("missing signature should not verify")
	}
	if VerifySignature(secret, body, "notsha256prefixed") {
		t.Error("malformed header should not verify")
	}
}

func TestParsePushAndBranch(t *testing.T) {
	body := []byte(`{
		"ref": "refs/heads/main",
		"after": "abc123",
		"repository": {"full_name":"acme/api","html_url":"https://github.com/acme/api","clone_url":"https://github.com/acme/api.git"}
	}`)
	p, err := ParsePush(body)
	if err != nil {
		t.Fatalf("ParsePush: %v", err)
	}
	if p.Branch() != "main" {
		t.Errorf("Branch() = %q, want main", p.Branch())
	}
	if p.After != "abc123" || p.Repository.FullName != "acme/api" {
		t.Errorf("unexpected parse: %+v", p)
	}

	tag, _ := ParsePush([]byte(`{"ref":"refs/tags/v1"}`))
	if tag.Branch() != "" {
		t.Errorf("tag ref should yield empty branch, got %q", tag.Branch())
	}
}
