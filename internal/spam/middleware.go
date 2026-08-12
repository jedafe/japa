package spam

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
)

// IPBlockMiddleware checks whether the request IP is blocked for the
// current tenant. Blocked IPs receive a 403 Forbidden response. The
// middleware extracts the real client IP via clientip.FromRequest, which
// honours X-Forwarded-For / X-Real-IP only when the immediate peer is a
// trusted proxy (JC_TRUSTED_PROXIES) — otherwise a blocked client could
// unblock itself by sending a spoofed forwarding header. The resolved IP
// is checked against the tenant's active IP blocks using PostgreSQL's
// <<= inet containment operator.
func IPBlockMiddleware(database *db.DB) func(http.Handler) http.Handler {
	store := NewStoreWithReader(database)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractIP(r)
			if ip == "" {
				next.ServeHTTP(w, r)
				return
			}

			blocked, err := store.IsIPBlocked(r.Context(), t.ID, ip)
			if err != nil {
				// Log but don't block on check failure — degrade gracefully
				slog.Warn(
					"ip block check failed",
					"ip", ip,
					"tenant_id", t.ID,
					"error", err,
				)
				next.ServeHTTP(w, r)

				return
			}

			if blocked {
				slog.Info(
					"blocked request from blocked IP",
					"ip", ip,
					"tenant_id", t.ID,
					"path", r.URL.Path,
				)
				writeSpamError(w, http.StatusForbidden, "ip_blocked", "Your IP address has been blocked")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CensorMiddleware provides a reusable censor check for content creation
// and update endpoints. It reads the request body, checks it against
// the tenant's censored word list, and blocks or flags the request as
// appropriate.
//
// This middleware is applied per-route via chi's .With() pattern, not
// globally. It reads the request body and passes the censor result via
// context so handlers can use it.
type contextKey struct{ name string }

// CensorResultKey is the context key for the censor check result.
var CensorResultKey = contextKey{"censor_result"}

// CensorMiddleware wraps a handler to check request JSON body for censored
// words before the handler executes. It expects the request body to contain
// a "body" or "content" field with the text to check.
func CensorMiddleware(database *db.DB, rdb *valkey.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := tenant.FromContext(r.Context())
			if t == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Extract text from the request body for censor checking
			text := extractBodyText(r)
			if text == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Get cached censored words (or fetch from DB on miss)
			words, err := GetCachedCensoredWords(r.Context(), rdb, database, t.ID)
			if err != nil {
				slog.Warn(
					"failed to load censored words for censor middleware",
					"tenant_id", t.ID,
					"error", err,
				)
				// Degrade gracefully — let the request through
				next.ServeHTTP(w, r)

				return
			}

			if len(words) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			result := CheckCensoredWords(words, text)

			if result.Blocked {
				slog.Info(
					"content blocked by censor middleware",
					"tenant_id", t.ID,
					"reason", result.FlagReason,
					"path", r.URL.Path,
				)
				writeSpamError(w, http.StatusForbidden, "content_blocked", result.FlagReason)

				return
			}

			// Store result in context for the handler to inspect
			ctx := context.WithValue(r.Context(), CensorResultKey, &result)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CensorResultFromContext extracts the CensorResult from the request context.
// Returns nil if no censor result is set (middleware was not applied or
// content was clean).
func CensorResultFromContext(ctx context.Context) *CensorResult {
	r, _ := ctx.Value(CensorResultKey).(*CensorResult)
	return r
}

// extractIP gets the client IP the IP blocklist is enforced on. Forwarding
// headers are only honoured when the peer is a trusted proxy, otherwise a
// blocked client could unblock itself with an X-Forwarded-For header.
func extractIP(r *http.Request) string {
	return clientip.FromRequest(r)
}

// extractBodyText reads the request body and extracts text content for
// censor checking. It looks for "body", "content", "title", and "message"
// fields in the JSON body. The body is read into a buffer and then
// restored on the request so the downstream handler can read it again —
// failing to restore the body would leave every handler behind this
// middleware with an empty/EOF body.
//
// The read is capped at 1MB via io.LimitReader. http.MaxBytesReader is
// not used because it requires a non-nil http.ResponseWriter to emit the
// 413 status on overflow, and this helper does not have one (passing nil
// panics on overflow).
func extractBodyText(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	// Read up to 1MB for censor checking. io.LimitReader does not panic
	// on overflow — it simply stops reading, so an oversized body is
	// truncated rather than causing a 413.
	limitedReader := io.LimitReader(r.Body, 1<<20)

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, limitedReader); err != nil {
		// Restore whatever was read so the handler can try too.
		r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		return ""
	}

	// Restore the body so the downstream handler can read it.
	r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))

	body := buf.String()
	if body == "" {
		return ""
	}

	// Try to extract text fields from JSON
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		// Not JSON — check the raw body
		return body
	}

	var text strings.Builder

	for _, field := range []string{"body", "content", "title", "message", "text"} {
		if val, ok := payload[field]; ok {
			if s, ok := val.(string); ok {
				text.WriteString(s)
				text.WriteString(" ")
			}
		}
	}

	return strings.TrimSpace(text.String())
}

// writeSpamError writes a JSON error response following the JapaConnect
// API error format.
func writeSpamError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}
