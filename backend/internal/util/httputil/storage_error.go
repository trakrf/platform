package httputil

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
)

// RespondStorageError classifies a storage-layer error by Postgres SQLSTATE
// and writes an appropriate RFC 7807 envelope.
//
// Currently handled:
//
//	23505 unique_violation -> 409 conflict
//
// All other codes (including wrapped non-pgx errors) fall through to
// 500 internal_error. 23503 (foreign_key_violation) is intentionally
// not mapped: the right status depends on whether the op was an insert
// (400/404) or a delete (409), which is out of TRA-407 scope.
func RespondStorageError(w http.ResponseWriter, r *http.Request, err error, requestID string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			WriteJSONError(w, r, http.StatusConflict, apierrors.ErrConflict,
				"Conflict", "Resource already exists", requestID)
			return
		}
	}
	WriteJSONError(w, r, http.StatusInternalServerError, apierrors.ErrInternal,
		"Internal Server Error", "An unexpected error occurred", requestID)
}
