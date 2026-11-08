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
// body larger than the JSON-API ceiling. Only multipart requests on routes
// that actually consume multipart are exempt — matching on Content-Type
// alone would let a spoofed multipart header smuggle an unbounded JSON
// body past the backstop. Every exempted path enforces its own tighter or
// equal cap downstream (MaxBytesReader before ParseMultipartForm).
//
// MAINTENANCE: when adding a new multipart upload route under /api/v1,
// add its path here — otherwise uploads over the JSON ceiling are rejected.
func isBulkBodyRequest(r *http.Request) bool {
	// Dev-only presigned upload sink (server.go mounts it outside /api/v1
	// but the check is harmless and future-proof if the path ever moves).
	if strings.HasPrefix(r.URL.Path, "/seaweedfs/upload/") {
		return true
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		return false
	}
	for _, p := range bulkBodyPrefixes {
		if strings.HasPrefix(r.URL.Path, p) {
			return true
		}
	}
	return false
}

// bulkBodyPrefixes are the /api/v1 routes whose handlers consume multipart
// bodies (each self-caps before parsing).
var bulkBodyPrefixes = []string{
	"/api/v1/videos/upload",
	"/api/v1/cv-formatter/format",
	"/api/v1/admin/universities/bulk-import",
	"/api/v1/admin/media/upload",
	"/api/v1/intel/inbound/email",
}
