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

// AccountUser represents an account_users junction table entry
type AccountUser struct {
	AccountID   int        `json:"account_id"`
	UserID      int        `json:"user_id"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
	Settings    any        `json:"settings"` // JSONB
	Metadata    any        `json:"metadata"` // JSONB
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// User details from JOIN
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

// AddUserToAccountRequest for POST /api/v1/accounts/:account_id/users
type AddUserToAccountRequest struct {
	UserID int    `json:"user_id" validate:"required"`
	Role   string `json:"role" validate:"required,oneof=owner admin member readonly"`
	Status string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}

// UpdateAccountUserRequest for PUT /api/v1/accounts/:account_id/users/:user_id
type UpdateAccountUserRequest struct {
	Role   *string `json:"role" validate:"omitempty,oneof=owner admin member readonly"`
	Status *string `json:"status" validate:"omitempty,oneof=active inactive suspended invited"`
}

// AccountUserListResponse for GET /api/v1/accounts/:account_id/users
type AccountUserListResponse struct {
	Data       []AccountUser `json:"data"`
	Pagination Pagination    `json:"pagination"`
}

var (
	ErrAccountUserNotFound  = errors.New("account user not found")
	ErrAccountUserDuplicate = errors.New("user already member of account")
)

// AccountUserRepository handles database operations for account_users
type AccountUserRepository struct {
	db *pgxpool.Pool
}

func (r *AccountUserRepository) List(ctx context.Context, accountID int, limit, offset int) ([]AccountUser, int, error) {
	// Query with JOIN to get user details
	query := `
		SELECT au.account_id, au.user_id, au.role, au.status, au.last_login_at,
		       au.settings, au.metadata, au.created_at, au.updated_at,
		       u.email, u.name
		FROM trakrf.account_users au
		INNER JOIN trakrf.users u ON au.user_id = u.id
		WHERE au.account_id = $1 AND au.deleted_at IS NULL
		ORDER BY au.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query account users: %w", err)
	}
	defer rows.Close()

	var accountUsers []AccountUser
	for rows.Next() {
		var au AccountUser
		err := rows.Scan(&au.AccountID, &au.UserID, &au.Role, &au.Status, &au.LastLoginAt,
			&au.Settings, &au.Metadata, &au.CreatedAt, &au.UpdatedAt,
			&au.UserEmail, &au.UserName)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan account user: %w", err)
		}
		accountUsers = append(accountUsers, au)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(*) FROM trakrf.account_users WHERE account_id = $1 AND deleted_at IS NULL"
	err = r.db.QueryRow(ctx, countQuery, accountID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count account users: %w", err)
	}

	return accountUsers, total, nil
}

func (r *AccountUserRepository) GetByID(ctx context.Context, accountID, userID int) (*AccountUser, error) {
	query := `
		SELECT au.account_id, au.user_id, au.role, au.status, au.last_login_at,
		       au.settings, au.metadata, au.created_at, au.updated_at,
		       u.email, u.name
		FROM trakrf.account_users au
		INNER JOIN trakrf.users u ON au.user_id = u.id
		WHERE au.account_id = $1 AND au.user_id = $2 AND au.deleted_at IS NULL
	`

	var au AccountUser
	err := r.db.QueryRow(ctx, query, accountID, userID).Scan(
		&au.AccountID, &au.UserID, &au.Role, &au.Status, &au.LastLoginAt,
		&au.Settings, &au.Metadata, &au.CreatedAt, &au.UpdatedAt,
		&au.UserEmail, &au.UserName)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get account user: %w", err)
	}

	return &au, nil
}

func (r *AccountUserRepository) Create(ctx context.Context, accountID int, req AddUserToAccountRequest) (*AccountUser, error) {
	// Set default status if not provided
	status := req.Status
	if status == "" {
		status = "active"
	}

	// Insert into account_users
	query := `
		INSERT INTO trakrf.account_users (account_id, user_id, role, status)
		VALUES ($1, $2, $3, $4)
		RETURNING account_id, user_id, role, status, last_login_at, settings, metadata, created_at, updated_at
	`

	var au AccountUser
	err := r.db.QueryRow(ctx, query, accountID, req.UserID, req.Role, status).Scan(
		&au.AccountID, &au.UserID, &au.Role, &au.Status, &au.LastLoginAt,
		&au.Settings, &au.Metadata, &au.CreatedAt, &au.UpdatedAt)

	if err != nil {
		// Check for unique constraint violation (duplicate membership)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrAccountUserDuplicate
		}
		return nil, fmt.Errorf("failed to create account user: %w", err)
	}

	// Fetch user details to populate response
	userQuery := "SELECT email, name FROM trakrf.users WHERE id = $1"
	err = r.db.QueryRow(ctx, userQuery, au.UserID).Scan(&au.UserEmail, &au.UserName)
	if err != nil {
		// Still return the account_user record even if user fetch fails
		// (shouldn't happen due to FK constraint)
		return &au, nil
	}

	return &au, nil
}

func (r *AccountUserRepository) Update(ctx context.Context, accountID, userID int, req UpdateAccountUserRequest) (*AccountUser, error) {
	// Build dynamic UPDATE query based on non-nil fields
	updates := []string{}
	args := []interface{}{accountID, userID}
	argPos := 3

	if req.Role != nil {
		updates = append(updates, fmt.Sprintf("role = $%d", argPos))
		args = append(args, *req.Role)
		argPos++
	}
	if req.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}

	if len(updates) == 0 {
		// No fields to update, just return current record
		return r.GetByID(ctx, accountID, userID)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.account_users
		SET %s, updated_at = NOW()
		WHERE account_id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING account_id, user_id, role, status, last_login_at, settings, metadata, created_at, updated_at
	`, strings.Join(updates, ", "))

	var au AccountUser
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&au.AccountID, &au.UserID, &au.Role, &au.Status, &au.LastLoginAt,
		&au.Settings, &au.Metadata, &au.CreatedAt, &au.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to update account user: %w", err)
	}

	// Fetch user details
	userQuery := "SELECT email, name FROM trakrf.users WHERE id = $1"
	r.db.QueryRow(ctx, userQuery, au.UserID).Scan(&au.UserEmail, &au.UserName)

	return &au, nil
}

func (r *AccountUserRepository) SoftDelete(ctx context.Context, accountID, userID int) error {
	query := `UPDATE trakrf.account_users SET deleted_at = NOW() WHERE account_id = $1 AND user_id = $2 AND deleted_at IS NULL`
	result, err := r.db.Exec(ctx, query, accountID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete account user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAccountUserNotFound
	}

	return nil
}

// HTTP Handlers

var accountUserRepo *AccountUserRepository

func initAccountUserRepo() {
	accountUserRepo = &AccountUserRepository{db: db}
}

func listAccountUsersHandler(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

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

	accountUsers, total, err := accountUserRepo.List(r.Context(), accountID, perPage, offset)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to list account users", "")
		return
	}

	resp := AccountUserListResponse{
		Data: accountUsers,
		Pagination: Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func addUserToAccountHandler(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	var req AddUserToAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	accountUser, err := accountUserRepo.Create(r.Context(), accountID, req)
	if err != nil {
		if errors.Is(err, ErrAccountUserDuplicate) {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "User already member of account", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to add user to account", "")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/accounts/%d/users/%d", accountID, req.UserID))
	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": accountUser})
}

func updateAccountUserHandler(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid user ID", "")
		return
	}

	var req UpdateAccountUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	accountUser, err := accountUserRepo.Update(r.Context(), accountID, userID, req)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to update account user", "")
		return
	}

	if accountUser == nil {
		writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account user not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": accountUser})
}

func removeUserFromAccountHandler(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.Atoi(chi.URLParam(r, "account_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "user_id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid user ID", "")
		return
	}

	if err := accountUserRepo.SoftDelete(r.Context(), accountID, userID); err != nil {
		if errors.Is(err, ErrAccountUserNotFound) {
			writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account user not found", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to remove user from account", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// registerAccountUserRoutes registers all account user endpoints (nested under accounts)
func registerAccountUserRoutes(r chi.Router) {
	r.Get("/api/v1/accounts/{account_id}/users", listAccountUsersHandler)
	r.Post("/api/v1/accounts/{account_id}/users", addUserToAccountHandler)
	r.Put("/api/v1/accounts/{account_id}/users/{user_id}", updateAccountUserHandler)
	r.Delete("/api/v1/accounts/{account_id}/users/{user_id}", removeUserFromAccountHandler)
}
