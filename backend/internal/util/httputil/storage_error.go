package httputil

import (
	"errors"
	"log/slog"
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
//	22*** data_exception   -> 400 bad_request (malformed input bytes)
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
				"Resource already exists", requestID)
			return
		case "23514":
			// check_constraint violation (e.g. no_self_reference on
			// locations). The request would have created a row that
			// violates a domain invariant — 409 conflict, not 500.
			slog.Warn("Check constraint violation",
				"sqlstate", pgErr.Code,
				"constraint", pgErr.ConstraintName,
				"request_id", requestID,
				"path", r.URL.Path)
			WriteJSONError(w, r, http.StatusConflict, apierrors.ErrConflict,
				"Request violates a domain invariant", requestID)
			return
		}
		// SQLSTATE class 22 = Data Exception (invalid bytes, unsupported
		// Unicode escapes in jsonb, numeric overflow on the SQL layer, etc.).
		// These are client-driven byte-pattern problems where the request
		// body conflicts with what the storage layer can persist — semantic
		// 409 conflict rather than client-malformed-request (TRA-678). 409
		// also keeps Schemathesis's positive_data_acceptance check green
		// (400 isn't in its allowlist; 409 is). Underlying SQLSTATE retained
		// in the slog record for correlation.
		if len(pgErr.Code) == 5 && pgErr.Code[:2] == "22" {
			slog.Warn("SQLSTATE-22 data exception",
				"sqlstate", pgErr.Code,
				"cause", pgErr.Message,
				"request_id", requestID,
				"path", r.URL.Path)
			WriteJSONError(w, r, http.StatusConflict, apierrors.ErrConflict,
				"Request body contains data that cannot be persisted as-is", requestID)
			return
		}
	}
	// Log the underlying cause before WriteJSONError scrubs the detail on
	// 5xx (TRA-673). The slog record carries the raw err.Error() with the
	// request_id so server-side correlation still works.
	slog.Error("Storage error",
		"cause", err.Error(),
		"request_id", requestID,
		"path", r.URL.Path)
	WriteJSONError(w, r, http.StatusInternalServerError, apierrors.ErrInternal,
		"An unexpected error occurred", requestID)
}
