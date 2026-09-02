// Package errors — HTTP response helpers for the canonical error envelope.
//
// These functions centralize the sentinel→HTTP-status mapping so that every
// handler package returns the same status code and machine-readable error
// code for the same sentinel error.  Callers should use HandleServiceError
// for service-layer errors and WriteError / WriteJSON for ad-hoc responses.
//
// Ref: japaconnect-engineering/conventions/api-response-format.md
// Ref: japaconnect-engineering/AGENTS.md Rule 5
package errors

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// errorResponse is the JSON body returned for error responses.
// {"error": {"code": "...", "message": "..."}}.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON serializes the given value as JSON and writes it to the
// response with the given status code. It sets the Content-Type header
// to application/json. If serialization fails it logs the error so a
// truncated response is diagnosable. A nil value produces no body — this
// is correct for 204 No Content and other nil-body responses, avoiding
// the incorrect "null" payload that json.Encode would emit.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding JSON response", "error", err, "status", status)
	}
}

// WriteError writes a JSON error response following the JapaConnect API
// error format. The code is a machine-readable snake_case string the
// frontend can switch on; the message is a human-readable explanation.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorResponse{
		Error: errorBody{
			Code:    code,
			Message: message,
		},
	})
}

// stripSentinelPrefix removes the sentinel error's message prefix from
// a wrapped error message.  For fmt.Errorf("%w: detail", sentinel) it
// returns just "detail".  If the error message does not contain the
// sentinel prefix, the full message is returned unchanged.  When the
// error IS the bare sentinel (message == sentinel message), the sentinel
// message itself is returned so the caller never gets an empty string.
func stripSentinelPrefix(err error, sentinel error) string {
	msg := err.Error()
	prefix := sentinel.Error()
	if !strings.HasPrefix(msg, prefix) {
		return msg
	}
	rest := strings.TrimPrefix(msg, prefix)
	// Strip the common ": " separator, then any leading whitespace.
	rest = strings.TrimPrefix(rest, ": ")
	rest = strings.TrimLeft(rest, " ")
	if rest == "" {
		return prefix
	}
	return rest
}

// HandleServiceError translates a service-layer error into the canonical
// HTTP response for the JapaConnect API.  It checks every sentinel
// defined in this package plus pgx.ErrNoRows, so callers do not need to
// enumerate sentinels individually.
//
// Handlers that have additional package-specific error sentinels should
// check those first, then call this function for the shared set.
//
// The mapping is:
//
//	ErrNotFound / pgx.ErrNoRows        → 404 not_found
//	ErrDuplicate / ErrAlreadyExists    → 409 already_exists
//	ErrRequired                        → 400 validation_error
//	ErrInvalid                         → 400 validation_error
//	ErrForbidden                       → 403 forbidden
//	ErrBelongsToAnother                → 403 forbidden
//	ErrExpired                         → 401 expired
//	ErrMaxDepth                        → 400 depth_exceeded
//	ErrInvalidStatusTransition         → 409 invalid_status_transition
//	ErrInsufficientBalance             → 422 insufficient_balance
//	ErrNotImplemented                  → 501 not_implemented
//	default                            → 500 internal_error
func HandleServiceError(w http.ResponseWriter, err error) {
	switch {
	// --- 404 Not Found ---
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, ErrFeatureDisabled):
		WriteError(w, http.StatusNotFound, "feature_disabled", "Feature is disabled")

	// --- 409 Conflict ---
	case errors.Is(err, ErrDuplicate), errors.Is(err, ErrAlreadyExists):
		WriteError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case errors.Is(err, ErrInvalidStatusTransition):
		WriteError(w, http.StatusConflict, "invalid_status_transition", stripSentinelPrefix(err, ErrInvalidStatusTransition))
	case errors.Is(err, ErrInUse):
		WriteError(w, http.StatusConflict, "in_use", stripSentinelPrefix(err, ErrInUse))

	// --- 400 Bad Request ---
	case errors.Is(err, ErrRequired):
		WriteError(w, http.StatusBadRequest, "validation_error", stripSentinelPrefix(err, ErrRequired))
	case errors.Is(err, ErrInvalid):
		WriteError(w, http.StatusBadRequest, "validation_error", stripSentinelPrefix(err, ErrInvalid))
	case errors.Is(err, ErrMaxDepth):
		WriteError(w, http.StatusBadRequest, "depth_exceeded", stripSentinelPrefix(err, ErrMaxDepth))

	// --- 403 Forbidden ---
	case errors.Is(err, ErrForbidden):
		WriteError(w, http.StatusForbidden, "forbidden", stripSentinelPrefix(err, ErrForbidden))
	case errors.Is(err, ErrBelongsToAnother):
		WriteError(w, http.StatusForbidden, "forbidden", stripSentinelPrefix(err, ErrBelongsToAnother))

	// --- 401 Unauthorized ---
	case errors.Is(err, ErrExpired):
		WriteError(w, http.StatusUnauthorized, "expired", stripSentinelPrefix(err, ErrExpired))

	// --- 422 Unprocessable Entity ---
	case errors.Is(err, ErrInsufficientBalance):
		WriteError(w, http.StatusUnprocessableEntity, "insufficient_balance", stripSentinelPrefix(err, ErrInsufficientBalance))
	case errors.Is(err, ErrScoringDisabled):
		WriteError(w, http.StatusUnprocessableEntity, "scoring_disabled", stripSentinelPrefix(err, ErrScoringDisabled))

	// --- 501 Not Implemented ---
	case errors.Is(err, ErrNotImplemented):
		WriteError(w, http.StatusNotImplemented, "not_implemented", stripSentinelPrefix(err, ErrNotImplemented))

	// --- 500 Internal Server Error ---
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred. Please try again later.")
	}
}
