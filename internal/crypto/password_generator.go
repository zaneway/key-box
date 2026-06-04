package crypto

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
)

const (
	lowercaseChars = "abcdefghijklmnopqrstuvwxyz"
	uppercaseChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitChars     = "0123456789"
	symbolChars    = "!@#$%^&*()-_=+[]{}:,.?"
	ambiguousChars = "0O1Il"
)

type PasswordGeneratorOptions struct {
	Length      int
	Lowercase   bool
	Uppercase   bool
	Digits      bool
	Symbols     bool
	NoAmbiguous bool
}

func GeneratePassword(options PasswordGeneratorOptions) (string, error) {
	if options.Length < 8 {
		return "", errors.New("password length must be at least 8")
	}

	classes := selectedCharacterClasses(options)
	if len(classes) == 0 {
		return "", errors.New("at least one character class must be selected")
	}
	if len(classes) > options.Length {
		return "", errors.New("password length is shorter than selected character classes")
	}

	var allChars string
	password := make([]byte, 0, options.Length)
	for _, class := range classes {
		allChars += class
		ch, err := randomChar(class)
		if err != nil {
			return "", err
		}
		password = append(password, ch)
	}

	for len(password) < options.Length {
		ch, err := randomChar(allChars)
		if err != nil {
			return "", err
		}
		password = append(password, ch)
	}

	if err := shuffleBytes(password); err != nil {
		return "", err
	}
	return string(password), nil
}

func selectedCharacterClasses(options PasswordGeneratorOptions) []string {
	var classes []string
	if options.Lowercase {
		classes = append(classes, filterAmbiguous(lowercaseChars, options.NoAmbiguous))
	}
	if options.Uppercase {
		classes = append(classes, filterAmbiguous(uppercaseChars, options.NoAmbiguous))
	}
	if options.Digits {
		classes = append(classes, filterAmbiguous(digitChars, options.NoAmbiguous))
	}
	if options.Symbols {
		classes = append(classes, symbolChars)
	}
	return classes
}

func filterAmbiguous(chars string, noAmbiguous bool) string {
	if !noAmbiguous {
		return chars
	}
	var b strings.Builder
	for _, ch := range chars {
		if !strings.ContainsRune(ambiguousChars, ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func randomChar(chars string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
	if err != nil {
		return 0, err
	}
	return chars[n.Int64()], nil
}

func shuffleBytes(values []byte) error {
	for i := len(values) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(n.Int64())
		values[i], values[j] = values[j], values[i]
	}
	return nil
}
