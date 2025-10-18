package main

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10 // Match Next.js implementation (trakrf-web)

// HashPassword generates bcrypt hash from plain text password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// ComparePassword checks if password matches hash
func ComparePassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("password comparison failed: %w", err)
	}
	return nil
}
