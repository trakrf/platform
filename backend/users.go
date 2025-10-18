package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User represents a user entity
type User struct {
	ID           int        `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"` // Never expose in JSON
	LastLoginAt  *time.Time `json:"last_login_at"`
	Settings     any        `json:"settings"` // JSONB
	Metadata     any        `json:"metadata"` // JSONB
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateUserRequest for POST /api/v1/users
type CreateUserRequest struct {
	Email        string `json:"email" validate:"required,email"`
	Name         string `json:"name" validate:"required,min=1,max=255"`
	PasswordHash string `json:"password_hash" validate:"required,min=8"` // Temporary, Phase 5 will hash
}

// UpdateUserRequest for PUT /api/v1/users/:id
type UpdateUserRequest struct {
	Name  *string `json:"name" validate:"omitempty,min=1,max=255"`
	Email *string `json:"email" validate:"omitempty,email"`
}

// UserListResponse for GET /api/v1/users
type UserListResponse struct {
	Data       []User     `json:"data"`
	Pagination Pagination `json:"pagination"`
}

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDuplicateEmail = errors.New("email already exists")
)

// UserRepository handles database operations for users
type UserRepository struct {
	db *pgxpool.Pool
}

func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]User, int, error) {
	// Query with soft delete filter
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
		FROM trakrf.users
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
			&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}

	// Get total count
	var total int
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.users WHERE deleted_at IS NULL").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	return users, total, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int) (*User, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
		FROM trakrf.users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var u User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
		&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &u, nil
}

// GetByEmail retrieves a user by email address
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
		FROM trakrf.users
		WHERE email = $1 AND deleted_at IS NULL
	`

	var u User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
		&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &u, nil
}

func (r *UserRepository) Create(ctx context.Context, req CreateUserRequest) (*User, error) {
	// Database generates ID via trigger, use RETURNING
	query := `
		INSERT INTO trakrf.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`

	var u User
	err := r.db.QueryRow(ctx, query, req.Email, req.Name, req.PasswordHash).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
		&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		// Check for unique constraint violation (duplicate email)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrUserDuplicateEmail
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &u, nil
}

func (r *UserRepository) Update(ctx context.Context, id int, req UpdateUserRequest) (*User, error) {
	// Build dynamic UPDATE query based on non-nil fields
	updates := []string{}
	args := []interface{}{id}
	argPos := 2

	if req.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.Name)
		argPos++
	}
	if req.Email != nil {
		updates = append(updates, fmt.Sprintf("email = $%d", argPos))
		args = append(args, *req.Email)
		argPos++
	}

	if len(updates) == 0 {
		// No fields to update, just return current record
		return r.GetByID(ctx, id)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.users
		SET %s, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, email, name, password_hash, last_login_at, settings, metadata, created_at, updated_at
	`, strings.Join(updates, ", "))

	var u User
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.LastLoginAt,
		&u.Settings, &u.Metadata, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		// Check for unique constraint violation (duplicate email)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrUserDuplicateEmail
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return &u, nil
}

func (r *UserRepository) SoftDelete(ctx context.Context, id int) error {
	query := `UPDATE trakrf.users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// HTTP Handlers

var userRepo *UserRepository

func initUserRepo() {
	userRepo = &UserRepository{db: db}
}

func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	// Parse pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	offset := (page - 1) * perPage

	users, total, err := userRepo.List(r.Context(), perPage, offset)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to list users", "")
		return
	}

	resp := UserListResponse{
		Data: users,
		Pagination: Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid user ID", "")
		return
	}

	user, err := userRepo.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to get user", "")
		return
	}

	if user == nil {
		writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "User not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": user})
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	user, err := userRepo.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrUserDuplicateEmail) {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Email already exists", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to create user", "")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/users/%d", user.ID))
	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": user})
}

func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid user ID", "")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	user, err := userRepo.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrUserDuplicateEmail) {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Email already exists", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to update user", "")
		return
	}

	if user == nil {
		writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "User not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": user})
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid user ID", "")
		return
	}

	if err := userRepo.SoftDelete(r.Context(), id); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "User not found", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to delete user", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// registerUserRoutes registers all user endpoints
func registerUserRoutes(r chi.Router) {
	r.Get("/api/v1/users", listUsersHandler)
	r.Get("/api/v1/users/{id}", getUserHandler)
	r.Post("/api/v1/users", createUserHandler)
	r.Put("/api/v1/users/{id}", updateUserHandler)
	r.Delete("/api/v1/users/{id}", deleteUserHandler)
}
