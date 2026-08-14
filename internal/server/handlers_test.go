package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// errorResponse mirrors the canonical error envelope for test decoding.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// mockPinger is a test double for the Pinger interface. It returns a
// configurable error from Ping so tests can simulate both a healthy and
// an unhealthy database without a real PostgreSQL connection.
type mockPinger struct {
	err error
}

func (m mockPinger) Ping(_ context.Context) error {
	return m.err
}

// TestHealthCheck verifies that the liveness probe returns 200 with
// {"status":"ok"} regardless of database state. The health check must
// never depend on external services — it only confirms the process is alive.
func TestHealthCheck(t *testing.T) {
	t.Parallel()

	h := NewHandler(mockPinger{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HealthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status field = %q, want %q", body["status"], "ok")
	}
}

// TestReadinessCheck_DBReady verifies that the readiness probe returns 200
// with a connected database status when Ping succeeds.
func TestReadinessCheck_DBReady(t *testing.T) {
	t.Parallel()

	h := NewHandler(mockPinger{err: nil})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	h.ReadinessCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status field = %q, want %q", body["status"], "ok")
	}

	if body["database"] != "connected" {
		t.Errorf("database field = %q, want %q", body["database"], "connected")
	}
}

// TestReadinessCheck_DBNotReady verifies that the readiness probe returns
// 503 with a JSON error body when Ping fails. The load balancer should
// stop routing traffic to this instance but the process should not restart.
func TestReadinessCheck_DBNotReady(t *testing.T) {
	t.Parallel()

	h := NewHandler(mockPinger{err: errors.New("connection refused")})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	h.ReadinessCheck(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if body.Error.Code != "db_unavailable" {
		t.Errorf("error code = %q, want %q", body.Error.Code, "db_unavailable")
	}

	if body.Error.Message == "" {
		t.Error("error message is empty, want a human-readable message")
	}
}

// TestReadinessCheck_ContentType verifies that the readiness response sets
// the correct Content-Type header so clients know to parse JSON.
func TestReadinessCheck_ContentType(t *testing.T) {
	t.Parallel()

	h := NewHandler(mockPinger{err: nil})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	h.ReadinessCheck(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
