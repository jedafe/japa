// Package errors provides centralized error handling and external error
// tracking integration for JapaConnect. When a GlitchTip DSN is
// configured, exceptions are forwarded to the error tracking service with
// release metadata for easy triage. The package degrades gracefully when no
// DSN is set — all functions become no-ops so callers never need conditional
// logic.
package errors

import (
	"log/slog"
	"time"

	glitchtip "github.com/getsentry/sentry-go"
)

// InitGlitchTip initializes the GlitchTip SDK with the provided DSN and
// release identifier. If dsn is empty the function returns immediately — the
// rest of the application continues without error tracking. AttachStacktrace
// is enabled so every captured exception includes a full call stack.
func InitGlitchTip(dsn, release string) {
	if dsn == "" {
		slog.Info("error tracking disabled — no DSN configured")
		return
	}

	if err := glitchtip.Init(glitchtip.ClientOptions{
		Dsn:              dsn,
		Release:          release,
		AttachStacktrace: true,
	}); err != nil {
		slog.Error("failed to initialize error tracking", "error", err)
		return
	}

	slog.Info("error tracking initialized", "release", release)

	// Fire a canary event to verify the DSN is valid and the GlitchTip
	// server is reachable. This surfaces configuration errors at boot
	// rather than silently failing until the first real exception.
	CaptureMessage("jc:bootstrap — GlitchTip initialized successfully")
}

// CaptureException forwards an error to the configured error tracking service.
// Nil errors are silently ignored. The event is enriched with the current
// scope (user, tags, extras) set by the caller before invoking this function.
func CaptureException(err error) {
	if err != nil {
		glitchtip.WithScope(func(_ *glitchtip.Scope) {
			glitchtip.CaptureException(err)
		})
	}
}

// CaptureMessage sends a plain-text message to the error tracking service.
// Use this for non-error events that still warrant visibility in the error
// tracker, such as degraded dependencies or rate-limited requests.
func CaptureMessage(msg string) {
	glitchtip.CaptureMessage(msg)
}

// Flush forces the GlitchTip SDK to send any buffered events to the server
// before the process exits. The timeout is in seconds. Call this during
// graceful shutdown to avoid losing events in process-termination races.
func Flush(timeout int) {
	glitchtip.Flush(time.Duration(timeout) * time.Second)
}
