package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/trakrf/platform/backend/internal/models/errors"
)

type ErrorResponse struct {
	Error struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Status    int    `json:"status"`
		Detail    string `json:"detail"`
		Instance  string `json:"instance"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

// WriteJSONError writes a standardized error response in RFC 7807 format.
func WriteJSONError(w http.ResponseWriter, r *http.Request, status int, errType errors.ErrorType, title, detail, requestID string) {
	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = title
	resp.Error.Status = status
	resp.Error.Detail = detail
	resp.Error.Instance = r.URL.Path
	resp.Error.RequestID = requestID

	if status >= 500 {
		slog.Error("Error response",
			"status", status,
			"type", errType,
			"detail", detail,
			"request_id", requestID,
			"path", r.URL.Path)
	} else {
		slog.Info("Client error",
			"status", status,
			"type", errType,
			"request_id", requestID,
			"path", r.URL.Path)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// WriteJSON writes a successful JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}
