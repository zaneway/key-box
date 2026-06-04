package crypto

import (
	"bytes"
	"testing"
)

func TestPasswordVerifierAcceptsCorrectPasswordOnly(t *testing.T) {
	verifier, err := NewPasswordVerifier("correct horse battery staple")
	if err != nil {
		t.Fatalf("NewPasswordVerifier() error = %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", verifier)
	if err != nil {
		t.Fatalf("VerifyPassword(correct) error = %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword(correct) = false, want true")
	}

	ok, err = VerifyPassword("wrong password", verifier)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong) error = %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword(wrong) = true, want false")
	}
}

func TestPasswordVerifierUsesRandomSaltAndDoesNotStorePlaintext(t *testing.T) {
	first, err := NewPasswordVerifier("same-password")
	if err != nil {
		t.Fatalf("first NewPasswordVerifier() error = %v", err)
	}
	second, err := NewPasswordVerifier("same-password")
	if err != nil {
		t.Fatalf("second NewPasswordVerifier() error = %v", err)
	}

	if first.KDF == "" {
		t.Fatal("KDF is empty")
	}
	if bytes.Equal(first.Salt, second.Salt) {
		t.Fatal("two verifiers for the same password used the same salt")
	}
	if bytes.Equal(first.Verifier, second.Verifier) {
		t.Fatal("two verifiers for the same password produced the same verifier")
	}
	if bytes.Contains(first.Verifier, []byte("same-password")) {
		t.Fatal("verifier contains plaintext password")
	}
}
