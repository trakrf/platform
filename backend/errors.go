package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorType represents the type of error
type ErrorType string

const (
	ErrValidation   ErrorType = "validation_error"
	ErrNotFound     ErrorType = "not_found"
	ErrConflict     ErrorType = "conflict"
	ErrInternal     ErrorType = "internal_error"
	ErrBadRequest   ErrorType = "bad_request"
	ErrUnauthorized ErrorType = "unauthorized" // Phase 5
)

// ErrorResponse implements RFC 7807 Problem Details
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

// writeJSONError writes a standardized error response
func writeJSONError(w http.ResponseWriter, r *http.Request, status int, errType ErrorType, title, detail string) {
	requestID := getRequestID(r.Context())

	resp := ErrorResponse{}
	resp.Error.Type = string(errType)
	resp.Error.Title = title
	resp.Error.Status = status
	resp.Error.Detail = detail
	resp.Error.Instance = r.URL.Path
	resp.Error.RequestID = requestID

	// Log errors based on severity
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

// writeJSON writes a successful JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}
