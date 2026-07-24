// Package server — HTTP handlers for infrastructure endpoints.
//
// The Handler struct holds dependencies shared across all handlers. Today
// the only dependency is the database pool, used by the readiness check.
// As more endpoints are added, this struct gains fields — handlers never
// reach for global state.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jedafe/japa/internal/httpx"
)

// Pinger is the dependency the readiness check needs from the database.
// *db.DB satisfies this interface, but tests can inject a mock so the
// readiness handler is testable without a live PostgreSQL instance.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler holds dependencies for all HTTP handlers in the API.
// Every handler method is a method on this struct so that dependencies
// are injected via the constructor, not read from globals.
type Handler struct {
	db Pinger
}

// NewHandler creates a Handler with the given database pool. The pool is
// used by the readiness check to verify the database is reachable. Any
// type implementing Pinger — including *db.DB – is accepted.
func NewHandler(db Pinger) *Handler {
	return &Handler{db: db}
}

// HealthCheck is the liveness probe. It returns 200 with {"status":"ok"}
// as long as the process is running and can serve requests. It does not
// check external dependencies — that is the job of ReadinessCheck.
//
// Kubernetes and load balancers use liveness probes to decide whether to
// restart the process; a failing liveness check means "this process is
// stuck and needs to be killed.".
func (h *Handler) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ReadinessCheck is the readiness probe. It pings the database to verify
// the application can serve real requests. If the database is unreachable
// it returns 503 so the load balancer stops sending traffic to this
// instance without restarting the process.
//
// The ping has a short timeout so a slow database does not cause the
// readiness check to hang indefinitely.
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "database is not ready")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"database": "connected",
	})
}

// errorResponse is the JSON body returned for error responses.
// It follows the JapaConnect API error format:
// {"error": {"code": "...", "message": "..."}}.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
