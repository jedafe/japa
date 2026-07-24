// Package httpx provides shared HTTP helpers for writing JSON responses and
// parsing common request parameters. It exists so handler packages do not each
// re-implement the same response envelope and parameter parsing logic.
package httpx

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrorBody is the payload describing a single API error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the JSON envelope returned for error responses.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// WriteJSON writes v as a JSON response with the given status code. A nil v
// writes only the status line, which keeps bodiless responses such as
// 204 No Content empty.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if v == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status header is already written, so the status code can no
		// longer be changed; log instead of failing the request.
		slog.Warn("httpx: json encode error", "error", err)
	}
}

// WriteError writes a standard error envelope with the given status code.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

// ParseUUIDParam reads the named chi URL parameter and parses it as a UUID.
func ParseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// ParseOffset reads the "offset" query parameter, returning 0 when it is
// missing, malformed, negative or out of int32 range.
func ParseOffset(r *http.Request) int32 {
	v := r.URL.Query().Get("offset")
	if v == "" {
		return 0
	}

	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > math.MaxInt32 {
		return 0
	}

	return int32(n)
}
