package crypto

import (
	"strings"
	"testing"
)

func TestGeneratePasswordHonorsLengthAndCharacterClasses(t *testing.T) {
	password, err := GeneratePassword(PasswordGeneratorOptions{
		Length:      24,
		Lowercase:   true,
		Uppercase:   true,
		Digits:      true,
		Symbols:     true,
		NoAmbiguous: true,
	})
	if err != nil {
		t.Fatalf("GeneratePassword() error = %v", err)
	}
	if len(password) != 24 {
		t.Fatalf("len(password) = %d, want 24", len(password))
	}

	assertContainsAny(t, password, lowercaseChars)
	assertContainsAny(t, password, uppercaseChars)
	assertContainsAny(t, password, digitChars)
	assertContainsAny(t, password, symbolChars)

	for _, ch := range ambiguousChars {
		if strings.ContainsRune(password, ch) {
			t.Fatalf("password contains ambiguous character %q: %q", ch, password)
		}
	}
}

func TestGeneratePasswordRejectsInvalidOptions(t *testing.T) {
	if _, err := GeneratePassword(PasswordGeneratorOptions{Length: 4, Lowercase: true}); err == nil {
		t.Fatal("GeneratePassword(short) error = nil, want error")
	}
	if _, err := GeneratePassword(PasswordGeneratorOptions{Length: 16}); err == nil {
		t.Fatal("GeneratePassword(no classes) error = nil, want error")
	}
}

func assertContainsAny(t *testing.T, password, chars string) {
	t.Helper()

	for _, ch := range password {
		if strings.ContainsRune(chars, ch) {
			return
		}
	}
	t.Fatalf("password %q does not contain any of %q", password, chars)
}
