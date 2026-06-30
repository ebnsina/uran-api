package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("supersecret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "supersecret") {
		t.Error("CheckPassword should accept the correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should reject an incorrect password")
	}
}

func TestAPIToken(t *testing.T) {
	tok, err := NewAPIToken()
	if err != nil {
		t.Fatalf("NewAPIToken: %v", err)
	}
	if len(tok) <= len(APITokenPrefix) || tok[:len(APITokenPrefix)] != APITokenPrefix {
		t.Errorf("token %q missing prefix", tok)
	}
	// Hash is deterministic and stable in length (sha256 hex = 64 chars).
	h1, h2 := HashAPIToken(tok), HashAPIToken(tok)
	if h1 != h2 || len(h1) != 64 {
		t.Errorf("hash unstable or wrong length: %q", h1)
	}
	if HashAPIToken("other") == h1 {
		t.Error("different tokens must hash differently")
	}
}

func TestNewTokenUnique(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	b, _ := NewToken()
	if a == b {
		t.Error("tokens should be unique")
	}
	if len(a) != 64 { // 32 bytes hex-encoded
		t.Errorf("token length = %d, want 64", len(a))
	}
}
