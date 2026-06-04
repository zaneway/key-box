package auth

import (
	"encoding/base32"
	"path/filepath"
	"testing"
	"time"

	"key-box/internal/config"
	"key-box/internal/crypto"
	"key-box/internal/db"
)

func TestRegisterThenLoginWithOTP(t *testing.T) {
	service := newTestService(t)

	result, err := service.Register("alice", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	code := otpFromBase32Secret(t, result.SecretKeyBBase32)
	keyC, err := service.Login("alice", code)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if len(keyC) != 32 {
		t.Fatalf("len(keyC) = %d, want 32", len(keyC))
	}
}

func TestRegisterWithPasswordRequiresPasswordAndOTP(t *testing.T) {
	service := newTestService(t)

	result, err := service.RegisterWithPassword("alice", "login-password", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("RegisterWithPassword() error = %v", err)
	}

	requiresSetup, err := service.RequiresPasswordSetup("alice")
	if err != nil {
		t.Fatalf("RequiresPasswordSetup() error = %v", err)
	}
	if requiresSetup {
		t.Fatal("RequiresPasswordSetup() = true for password account, want false")
	}

	code := otpFromBase32Secret(t, result.SecretKeyBBase32)
	keyC, err := service.LoginWithPassword("alice", "login-password", code)
	if err != nil {
		t.Fatalf("LoginWithPassword(correct) error = %v", err)
	}
	if len(keyC) != 32 {
		t.Fatalf("len(keyC) = %d, want 32", len(keyC))
	}

	if _, err := service.LoginWithPassword("alice", "wrong-password", code); err == nil {
		t.Fatal("LoginWithPassword(wrong password) error = nil, want error")
	}
	if _, err := service.LoginWithPassword("alice", "login-password", "000000"); err == nil {
		t.Fatal("LoginWithPassword(wrong OTP) error = nil, want error")
	}
}

func TestLegacyAccountCanSetPasswordAfterOTPLogin(t *testing.T) {
	service := newTestService(t)

	result, err := service.Register("alice", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	requiresSetup, err := service.RequiresPasswordSetup("alice")
	if err != nil {
		t.Fatalf("RequiresPasswordSetup() error = %v", err)
	}
	if !requiresSetup {
		t.Fatal("RequiresPasswordSetup() = false for legacy account, want true")
	}

	code := otpFromBase32Secret(t, result.SecretKeyBBase32)
	if _, err := service.Login("alice", code); err != nil {
		t.Fatalf("legacy OTP Login() error = %v", err)
	}
	if err := service.SetLoginPassword("alice", "new-login-password"); err != nil {
		t.Fatalf("SetLoginPassword() error = %v", err)
	}

	requiresSetup, err = service.RequiresPasswordSetup("alice")
	if err != nil {
		t.Fatalf("RequiresPasswordSetup() after set error = %v", err)
	}
	if requiresSetup {
		t.Fatal("RequiresPasswordSetup() after set = true, want false")
	}
	if _, err := service.LoginWithPassword("alice", "new-login-password", code); err != nil {
		t.Fatalf("LoginWithPassword() after setup error = %v", err)
	}
}

func TestResetPasswordWithNewLoginPassword(t *testing.T) {
	service := newTestService(t)

	result, err := service.RegisterWithPassword("alice", "old-login-password", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("RegisterWithPassword() error = %v", err)
	}
	oldCode := otpFromBase32Secret(t, result.SecretKeyBBase32)

	resetResult, err := service.ResetPasswordWithLoginPassword("alice", "a1", "a2", "a3", "new-login-password")
	if err != nil {
		t.Fatalf("ResetPasswordWithLoginPassword() error = %v", err)
	}
	newCode := otpFromBase32Secret(t, resetResult.SecretKeyBBase32)

	if _, err := service.LoginWithPassword("alice", "old-login-password", oldCode); err == nil {
		t.Fatal("LoginWithPassword(old credentials) error = nil, want error")
	}
	if _, err := service.LoginWithPassword("alice", "new-login-password", newCode); err != nil {
		t.Fatalf("LoginWithPassword(new credentials) error = %v", err)
	}
}

func TestLoginFailsWhenSaltDoesNotMatch(t *testing.T) {
	service := newTestService(t)

	result, err := service.Register("alice", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := config.SaveSalt("different-salt"); err != nil {
		t.Fatalf("SaveSalt(different) error = %v", err)
	}

	code := otpFromBase32Secret(t, result.SecretKeyBBase32)
	_, err = service.Login("alice", code)
	if err == nil {
		t.Fatal("Login() error = nil, want root key mismatch error")
	}
}

func TestResetPasswordRotatesOTPSecret(t *testing.T) {
	service := newTestService(t)

	result, err := service.Register("alice", "q1", "q2", "q3", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	oldCode := otpFromBase32Secret(t, result.SecretKeyBBase32)

	resetResult, err := service.ResetPassword("alice", "a1", "a2", "a3")
	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if resetResult.SecretKeyBBase32 == result.SecretKeyBBase32 {
		t.Fatal("ResetPassword() returned the same OTP secret")
	}

	if _, err := service.Login("alice", oldCode); err == nil {
		t.Fatal("Login() with old OTP error = nil, want invalid OTP")
	}

	newCode := otpFromBase32Secret(t, resetResult.SecretKeyBBase32)
	if _, err := service.Login("alice", newCode); err != nil {
		t.Fatalf("Login() with new OTP error = %v", err)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	if err := config.SaveSalt("test-root-salt"); err != nil {
		t.Fatalf("SaveSalt() error = %v", err)
	}

	database, err := db.InitDBAt(filepath.Join(t.TempDir(), "key-box.db"))
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	return NewService(database)
}

func otpFromBase32Secret(t *testing.T, secret string) string {
	t.Helper()

	keyB, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("decode Key B: %v", err)
	}
	return crypto.GenerateTOTP(keyB, time.Now())
}
