package password

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10

// Hash generates a bcrypt hash from a plain text password.
func Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// Compare checks if a plain text password matches a bcrypt hash.
func Compare(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password comparison failed: %w", err)
	}
	return nil
}
