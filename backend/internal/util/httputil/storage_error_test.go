package httputil_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

func TestRespondStorageError_UniqueViolationMapsTo409(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondStorageError(w, r, pgErr, "req-1")

	if w.Code != 409 {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Type != string(apierrors.ErrConflict) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrConflict)
	}
}

func TestRespondStorageError_WrappedPgxStillClassifies(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505"}
	wrapped := fmt.Errorf("storage: %w", pgErr)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondStorageError(w, r, wrapped, "req-1")

	if w.Code != 409 {
		t.Fatalf("status = %d, want 409 (wrapped pgx still classifies)", w.Code)
	}
}

func TestRespondStorageError_NonPgxMapsTo500(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondStorageError(w, r, errors.New("something broke"), "req-1")

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var resp apierrors.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Type != string(apierrors.ErrInternal) {
		t.Errorf("type = %q, want %q", resp.Error.Type, apierrors.ErrInternal)
	}
}

func TestRespondStorageError_OtherPgCodesMapTo500(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23503"} // foreign_key_violation — intentionally unmapped in TRA-407 scope.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	httputil.RespondStorageError(w, r, pgErr, "req-1")

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500 (23503 not classified in TRA-407 scope)", w.Code)
	}
}
