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
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Account represents an account entity
type Account struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Domain           string    `json:"domain"`
	Status           string    `json:"status"`
	SubscriptionTier string    `json:"subscription_tier"`
	MaxUsers         int       `json:"max_users"`
	MaxStorageGB     int       `json:"max_storage_gb"`
	Settings         any       `json:"settings"` // JSONB
	Metadata         any       `json:"metadata"` // JSONB
	BillingEmail     string    `json:"billing_email"`
	TechnicalEmail   *string   `json:"technical_email"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateAccountRequest for POST /api/v1/accounts
type CreateAccountRequest struct {
	Name             string  `json:"name" validate:"required,min=1,max=255"`
	Domain           string  `json:"domain" validate:"required,hostname"`
	BillingEmail     string  `json:"billing_email" validate:"required,email"`
	TechnicalEmail   *string `json:"technical_email" validate:"omitempty,email"`
	SubscriptionTier string  `json:"subscription_tier" validate:"omitempty,oneof=free basic premium god-mode"`
	MaxUsers         *int    `json:"max_users" validate:"omitempty,min=1"`
	MaxStorageGB     *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// UpdateAccountRequest for PUT /api/v1/accounts/:id
type UpdateAccountRequest struct {
	Name           *string `json:"name" validate:"omitempty,min=1,max=255"`
	BillingEmail   *string `json:"billing_email" validate:"omitempty,email"`
	TechnicalEmail *string `json:"technical_email" validate:"omitempty,email"`
	Status         *string `json:"status" validate:"omitempty,oneof=active inactive suspended"`
	MaxUsers       *int    `json:"max_users" validate:"omitempty,min=1"`
	MaxStorageGB   *int    `json:"max_storage_gb" validate:"omitempty,min=1"`
}

// AccountListResponse for GET /api/v1/accounts
type AccountListResponse struct {
	Data       []Account  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Pagination metadata
type Pagination struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

var validate = validator.New()

var (
	ErrAccountNotFound        = errors.New("account not found")
	ErrAccountDuplicateDomain = errors.New("domain already exists")
)

// AccountRepository handles database operations for accounts
type AccountRepository struct {
	db *pgxpool.Pool
}

func (r *AccountRepository) List(ctx context.Context, limit, offset int) ([]Account, int, error) {
	// Query with soft delete filter: WHERE deleted_at IS NULL
	query := `
		SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		       settings, metadata, billing_email, technical_email, created_at, updated_at
		FROM trakrf.accounts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		err := rows.Scan(&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
			&a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
			&a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, a)
	}

	// Get total count
	var total int
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM trakrf.accounts WHERE deleted_at IS NULL").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count accounts: %w", err)
	}

	return accounts, total, nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id int) (*Account, error) {
	query := `
		SELECT id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		       settings, metadata, billing_email, technical_email, created_at, updated_at
		FROM trakrf.accounts
		WHERE id = $1 AND deleted_at IS NULL
	`

	var a Account
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
		&a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
		&a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &a, nil
}

func (r *AccountRepository) Create(ctx context.Context, req CreateAccountRequest) (*Account, error) {
	// Database generates ID via trigger, use RETURNING
	query := `
		INSERT INTO trakrf.accounts (name, domain, billing_email, technical_email, subscription_tier, max_users, max_storage_gb)
		VALUES ($1, $2, $3, $4, COALESCE($5, 'free'), COALESCE($6, 5), COALESCE($7, 1))
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`

	var a Account
	err := r.db.QueryRow(ctx, query,
		req.Name, req.Domain, req.BillingEmail, req.TechnicalEmail,
		req.SubscriptionTier, req.MaxUsers, req.MaxStorageGB,
	).Scan(&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
		&a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
		&a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)

	if err != nil {
		// Check for unique constraint violation (duplicate domain)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, ErrAccountDuplicateDomain
		}
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return &a, nil
}

func (r *AccountRepository) Update(ctx context.Context, id int, req UpdateAccountRequest) (*Account, error) {
	// Build dynamic UPDATE query based on non-nil fields
	updates := []string{}
	args := []interface{}{id}
	argPos := 2

	if req.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argPos))
		args = append(args, *req.Name)
		argPos++
	}
	if req.BillingEmail != nil {
		updates = append(updates, fmt.Sprintf("billing_email = $%d", argPos))
		args = append(args, *req.BillingEmail)
		argPos++
	}
	if req.TechnicalEmail != nil {
		updates = append(updates, fmt.Sprintf("technical_email = $%d", argPos))
		args = append(args, *req.TechnicalEmail)
		argPos++
	}
	if req.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *req.Status)
		argPos++
	}
	if req.MaxUsers != nil {
		updates = append(updates, fmt.Sprintf("max_users = $%d", argPos))
		args = append(args, *req.MaxUsers)
		argPos++
	}
	if req.MaxStorageGB != nil {
		updates = append(updates, fmt.Sprintf("max_storage_gb = $%d", argPos))
		args = append(args, *req.MaxStorageGB)
		argPos++
	}

	if len(updates) == 0 {
		// No fields to update, just return current record
		return r.GetByID(ctx, id)
	}

	query := fmt.Sprintf(`
		UPDATE trakrf.accounts
		SET %s, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, domain, status, subscription_tier, max_users, max_storage_gb,
		          settings, metadata, billing_email, technical_email, created_at, updated_at
	`, strings.Join(updates, ", "))

	var a Account
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.Name, &a.Domain, &a.Status, &a.SubscriptionTier,
		&a.MaxUsers, &a.MaxStorageGB, &a.Settings, &a.Metadata,
		&a.BillingEmail, &a.TechnicalEmail, &a.CreatedAt, &a.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to update account: %w", err)
	}

	return &a, nil
}

func (r *AccountRepository) SoftDelete(ctx context.Context, id int) error {
	query := `UPDATE trakrf.accounts SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAccountNotFound
	}

	return nil
}

// HTTP Handlers

var accountRepo *AccountRepository

func initAccountRepo() {
	accountRepo = &AccountRepository{db: db}
}

func listAccountsHandler(w http.ResponseWriter, r *http.Request) {
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

	accounts, total, err := accountRepo.List(r.Context(), perPage, offset)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to list accounts", "")
		return
	}

	resp := AccountListResponse{
		Data: accounts,
		Pagination: Pagination{
			Page:    page,
			PerPage: perPage,
			Total:   total,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func getAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	account, err := accountRepo.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to get account", "")
		return
	}

	if account == nil {
		writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": account})
}

func createAccountHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	account, err := accountRepo.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrAccountDuplicateDomain) {
			writeJSONError(w, r, http.StatusConflict, ErrConflict, "Domain already exists", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to create account", "")
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/accounts/%d", account.ID))
	writeJSON(w, http.StatusCreated, map[string]interface{}{"data": account})
}

func updateAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	var req UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid JSON", err.Error())
		return
	}

	if err := validate.Struct(req); err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrValidation, "Validation failed", err.Error())
		return
	}

	account, err := accountRepo.Update(r.Context(), id, req)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to update account", "")
		return
	}

	if account == nil {
		writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account not found", "")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": account})
}

func deleteAccountHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONError(w, r, http.StatusBadRequest, ErrBadRequest, "Invalid account ID", "")
		return
	}

	if err := accountRepo.SoftDelete(r.Context(), id); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			writeJSONError(w, r, http.StatusNotFound, ErrNotFound, "Account not found", "")
			return
		}
		writeJSONError(w, r, http.StatusInternalServerError, ErrInternal, "Failed to delete account", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// registerAccountRoutes registers all account endpoints
func registerAccountRoutes(r chi.Router) {
	r.Get("/api/v1/accounts", listAccountsHandler)
	r.Get("/api/v1/accounts/{id}", getAccountHandler)
	r.Post("/api/v1/accounts", createAccountHandler)
	r.Put("/api/v1/accounts/{id}", updateAccountHandler)
	r.Delete("/api/v1/accounts/{id}", deleteAccountHandler)
}
