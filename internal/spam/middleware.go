package spam

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/valkey"
)

// IPBlockMiddleware checks whether the request IP is blocked for the
// current tenant. Per §41 IP Block Middleware, the block_type determines
// the scope of the block:
//   - "full"         → 403 Forbidden on every request (global block).
//   - "registration" → stored in context; enforced by the registration
//     route group via RequireNotIPBlocked("registration").
//   - "posting"      → stored in context; enforced by the content/comment
//     creation route group via RequireNotIPBlocked("posting").
//   - "api"          → stored in context; enforced by the API-key route
//     group via RequireNotIPBlocked("api").
//
// The middleware extracts the real client IP via clientip.FromRequest, which
// honours X-Forwarded-For / X-Real-IP only when the immediate peer is a
// trusted proxy (JC_TRUSTED_PROXIES) — otherwise a blocked client could
// unblock itself by sending a spoofed forwarding header. The resolved IP
// is checked against the tenant's active IP blocks using PostgreSQL's
// <<= inet containment operator.
func IPBlockMiddleware(database *db.DB) func(http.Handler) http.Handler {
	store := NewStoreWithReader(database)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := &struct{ ID pgtype.UUID }{}
			if t == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractIP(r)
			if ip == "" {
				next.ServeHTTP(w, r)
				return
			}

			blockType, err := store.GetIPBlockTypeForIP(r.Context(), ip)
			if err != nil {
				// Log but don't block on check failure — degrade gracefully
				slog.Warn("ip block check failed", "ip", ip, "error", err)
				next.ServeHTTP(w, r)

				return
			}

			if blockType == "" {
				next.ServeHTTP(w, r)
				return
			}

			// "full" blocks every request immediately. Narrower types are
			// stored in context so the relevant route group can enforce them.
			if blockType == BlockTypeFull {
				slog.Info("blocked request from blocked IP", "ip", ip, "path", r.URL.Path)
				writeSpamError(w, http.StatusForbidden, "ip_blocked", "Your IP address has been blocked")

				return
			}

			ctx := context.WithValue(r.Context(), ipBlockTypeKey, blockType)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ipBlockTypeKey is the context key for the per-type IP block flag.
var ipBlockTypeKey = contextKey{"ip_block_type"}

// IPBlockTypeFromContext returns the IP block type stored by
// IPBlockMiddleware ("registration", "posting", "api"), or "" if no
// narrower-than-full block is active for the request.
func IPBlockTypeFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ipBlockTypeKey).(string)
	return v
}

// RequireNotIPBlocked returns a middleware that 403s requests whose IP is
// flagged with the given block_type in context. Mount it on the route group
// that the block type governs (e.g. registration routes, content-creation
// routes, API-key routes). Per §41, a "full" block is already enforced
// globally by IPBlockMiddleware and never reaches this check.
func RequireNotIPBlocked(blockType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IPBlockTypeFromContext(r.Context()) == blockType {
				writeSpamError(w, http.StatusForbidden, "ip_blocked",
					"Your IP address has been blocked from this action")
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
//
// For action=block patterns the middleware 403s immediately. For action=replace
// patterns the middleware rewrites the request body with the cleaned text so
// the downstream handler persists the censored version (BUG-012). For
// action=flag patterns the middleware logs the flag and stores the result in
// context; the spam pipeline's Flagged path (which sets StatusInReview) is the
// canonical moderation-queue entry point, so the middleware does not enqueue a
// separate moderation task here.
func CensorMiddleware(database *db.DB, rdb *valkey.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := &struct{ ID pgtype.UUID }{}
			if t == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Extract text from the request body for censor checking.
			// extractBodyText also restores the body on the request so the
			// downstream handler can read it.
			text, payload, rawBody := extractBodyText(r)
			_ = rawBody // preserved for non-JSON bodies; not rewritten

			if text == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Get cached censored words (or fetch from DB on miss)
			words, err := GetCachedCensoredWords(r.Context(), rdb, database)
			if err != nil {
				slog.Warn("failed to load censored words for censor middleware", "error", err)
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
				slog.Info("content blocked by censor middleware", "reason", result.FlagReason, "path", r.URL.Path)
				writeSpamError(w, http.StatusForbidden, "content_blocked", result.FlagReason)

				return
			}

			// For replace actions, rewrite each text field in the JSON body
			// with its censored version so the downstream handler persists the
			// cleaned text (BUG-012). The censor check runs against the
			// concatenated text, but replacement is applied per-field by
			// re-running CheckCensoredWords on each field individually.
			if payload != nil && (result.CleanText != "" && result.CleanText != text) {
				if rewritten, ok := rewriteBodyFields(payload, words); ok {
					r.Body = io.NopCloser(bytes.NewReader(rewritten))
					r.ContentLength = int64(len(rewritten))
				}
			}

			if result.Flagged {
				slog.Info("content flagged by censor middleware", "reason", result.FlagReason, "path", r.URL.Path)
			}

			// Store result in context for the handler to inspect
			ctx := context.WithValue(r.Context(), CensorResultKey, &result)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// rewriteBodyFields applies censor replacements to each text field in the JSON
// payload individually and re-serializes the body. Returns false when no field
// was changed or re-serialization fails.
func rewriteBodyFields(payload map[string]any, words []CensoredWord) ([]byte, bool) {
	changed := false

	for _, field := range []string{"body", "content", "title", "message", "text"} {
		val, ok := payload[field]
		if !ok {
			continue
		}

		s, ok := val.(string)
		if !ok {
			continue
		}

		fieldResult := CheckCensoredWords(words, s)
		if fieldResult.Blocked {
			// A blocked field should have been caught earlier; skip rewrite.
			continue
		}

		if fieldResult.CleanText != s {
			payload[field] = fieldResult.CleanText
			changed = true
		}
	}

	if !changed {
		return nil, false
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}

	return rewritten, true
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
// censor checking. It looks for "body", "content", "title", "message", and
// "text" fields in the JSON body. The body is read into a buffer and then
// restored on the request so the downstream handler can read it again —
// failing to restore the body would leave every handler behind this
// middleware with an empty/EOF body.
//
// Returns the concatenated text from all text fields, the parsed JSON payload
// (nil for non-JSON bodies, used by the middleware to rewrite fields for
// replace actions), and the raw body string. The raw body is returned so
// callers that need the original bytes (e.g. for non-JSON censor checks) can
// access them without re-reading the request.
//
// The read is capped at 1MB via io.LimitReader. http.MaxBytesReader is
// not used because it requires a non-nil http.ResponseWriter to emit the
// 413 status on overflow, and this helper does not have one (passing nil
// panics on overflow).
func extractBodyText(r *http.Request) (text string, payload map[string]any, rawBody string) {
	if r.Body == nil {
		return "", nil, ""
	}

	// Read up to 1MB for censor checking. io.LimitReader does not panic
	// on overflow — it simply stops reading, so an oversized body is
	// truncated rather than causing a 413.
	limitedReader := io.LimitReader(r.Body, 1<<20)

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, limitedReader); err != nil {
		// Restore whatever was read so the handler can try too.
		r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
		return "", nil, buf.String()
	}

	// Restore the body so the downstream handler can read it.
	r.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))

	body := buf.String()
	if body == "" {
		return "", nil, ""
	}

	// Try to extract text fields from JSON
	var p map[string]any
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		// Not JSON — check the raw body
		return body, nil, body
	}

	var sb strings.Builder

	for _, field := range []string{"body", "content", "title", "message", "text"} {
		if val, ok := p[field]; ok {
			if s, ok := val.(string); ok {
				sb.WriteString(s)
				sb.WriteString(" ")
			}
		}
	}

	return strings.TrimSpace(sb.String()), p, body
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
