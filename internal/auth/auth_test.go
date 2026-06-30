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
