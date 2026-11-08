package middleware

import (
	"net/http"
	"strings"
)

// RequestBodyLimit caps request bodies for endpoints that don't set their
// own limit. It is the API-wide backstop for json.NewDecoder/ReadAll call
// sites, which would otherwise buffer an unbounded body.
//
// Multipart and raw-octet-stream uploads are exempt: those handlers set
// their own caps (MaxBytesReader before ParseMultipartForm) or stream to
// storage behind pending-upload records. Handlers may still re-wrap
// r.Body with a tighter MaxBytesReader — limits compose, the inner one wins.
func RequestBodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && !isBulkBodyRequest(r) {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isBulkBodyRequest reports whether the request legitimately carries a
// body larger than the JSON-API ceiling: multipart uploads and the
// presigned PUT fallback, which enforce their own size bounds downstream.
func isBulkBodyRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") || ct == "application/octet-stream" {
		return true
	}
	// Dev-only presigned upload sink (server.go mounts it outside /api/v1
	// but the check is harmless and future-proof if the path ever moves).
	return strings.HasPrefix(r.URL.Path, "/seaweedfs/upload/")
}
