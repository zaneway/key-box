package crypto

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	passwordKDF        = "pbkdf2-sha256:210000:32"
	passwordIterations = 210000
	passwordKeyLength  = 32
	passwordSaltLength = 16
)

type PasswordVerifier struct {
	Salt     []byte
	Verifier []byte
	KDF      string
}

func NewPasswordVerifier(password string) (*PasswordVerifier, error) {
	if password == "" {
		return nil, errors.New("password cannot be empty")
	}

	salt, err := GenerateRandomBytes(passwordSaltLength)
	if err != nil {
		return nil, err
	}

	return &PasswordVerifier{
		Salt:     salt,
		Verifier: derivePasswordVerifier(password, salt),
		KDF:      passwordKDF,
	}, nil
}

func VerifyPassword(password string, verifier *PasswordVerifier) (bool, error) {
	if verifier == nil {
		return false, errors.New("password verifier is nil")
	}
	if verifier.KDF != passwordKDF {
		return false, fmt.Errorf("unsupported password KDF: %s", verifier.KDF)
	}
	if len(verifier.Salt) == 0 || len(verifier.Verifier) == 0 {
		return false, errors.New("password verifier is incomplete")
	}

	candidate := derivePasswordVerifier(password, verifier.Salt)
	return subtle.ConstantTimeCompare(candidate, verifier.Verifier) == 1, nil
}

func derivePasswordVerifier(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, passwordIterations, passwordKeyLength, sha256.New)
}
