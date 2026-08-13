package auth

import (
	"context"
	"errors"
	"fmt"

	authmodels "github.com/trakrf/platform/backend/internal/models/auth"
)

// ErrInvalidCurrentPassword is returned by ChangePassword when the supplied
// current password does not match the stored hash. The handler maps it to a
// 400 rather than a 401 — the caller is authenticated, just wrong about the
// password.
var ErrInvalidCurrentPassword = errors.New("invalid_current_password")

// ChangePassword verifies the user's current password and replaces the stored
// hash with one of the new password (TRA-1130). Hash/compare functions are
// injected the same way Signup/Login take them.
func (s *Service) ChangePassword(ctx context.Context, userID int, request authmodels.ChangePasswordRequest, comparePassword func(string, string) error, hashPassword func(string) (string, error)) error {
	usr, err := s.storage.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to lookup user: %w", err)
	}
	if usr == nil {
		return fmt.Errorf("user not found")
	}

	if err := comparePassword(request.CurrentPassword, usr.PasswordHash); err != nil {
		return ErrInvalidCurrentPassword
	}

	passwordHash, err := hashPassword(request.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.storage.UpdateUserPassword(ctx, userID, passwordHash); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}
