package auth

import (
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	hasLetter := false
	hasNumber := false
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsNumber(r):
			hasNumber = true
		}
	}
	if !hasLetter || !hasNumber || strings.TrimSpace(password) != password {
		return ErrWeakPassword
	}
	return nil
}
