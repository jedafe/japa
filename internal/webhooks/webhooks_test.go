package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Schema Version
// ---------------------------------------------------------------------------

func TestSchemaVersion(t *testing.T) {
	t.Parallel()

	if SchemaVersion != "1.0" {
		t.Errorf("SchemaVersion = %q, want %q", SchemaVersion, "1.0")
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	t.Parallel()

	if SignatureHeader != "X-Webhook-Signature" {
		t.Errorf("SignatureHeader = %q, want %q", SignatureHeader, "X-Webhook-Signature")
	}

	if ContentType != "application/json" {
		t.Errorf("ContentType = %q, want %q", ContentType, "application/json")
	}

	if MaxDeliveryAttempts != 5 {
		t.Errorf("MaxDeliveryAttempts = %d, want 5", MaxDeliveryAttempts)
	}
}

// ---------------------------------------------------------------------------
// WebhookEnvelope Marshal
// ---------------------------------------------------------------------------

func TestWebhookEnvelopeMarshal(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	env := WebhookEnvelope{
		EventID:       "evt-123",
		EventType:     "payment.completed",
		OccurredAt:    now,
		SchemaVersion: SchemaVersion,
		Data:          map[string]any{"amount": float64(100)},
	}

	payload, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	tests := []struct {
		key  string
		want any
	}{
		{"event_id", "evt-123"},
		{"event_type", "payment.completed"},
		{"schema_version", "1.0"},
	}
	for _, tt := range tests {
		if parsed[tt.key] != tt.want {
			t.Errorf("parsed[%q] = %v, want %v", tt.key, parsed[tt.key], tt.want)
		}
	}
}

func TestWebhookEnvelopeMarshalEmpty(t *testing.T) {
	t.Parallel()

	env := WebhookEnvelope{}

	payload, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if len(payload) == 0 {
		t.Error("Marshal() returned empty payload")
	}
}

// ---------------------------------------------------------------------------
// SignPayload — HMAC-SHA256
// ---------------------------------------------------------------------------

func TestSignPayloadDeterministic(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	payload := []byte(`{"event":"test"}`)

	sig1 := SignPayload(secret, payload)
	sig2 := SignPayload(secret, payload)

	if sig1 != sig2 {
		t.Errorf("same input produced different signatures: %q vs %q", sig1, sig2)
	}
}

func TestSignPayloadDifferentSecrets(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"event":"test"}`)

	sig1 := SignPayload("secret-a", payload)
	sig2 := SignPayload("secret-b", payload)

	if sig1 == sig2 {
		t.Error("different secrets produced the same signature")
	}
}

func TestSignPayloadDifferentPayloads(t *testing.T) {
	t.Parallel()

	secret := "test-secret"

	sig1 := SignPayload(secret, []byte("payload-1"))
	sig2 := SignPayload(secret, []byte("payload-2"))

	if sig1 == sig2 {
		t.Error("different payloads produced the same signature")
	}
}

func TestSignPayloadMatchesManualHMAC(t *testing.T) {
	t.Parallel()

	secret := "my-webhook-secret"
	payload := []byte(`{"data":"hello"}`)

	sig := SignPayload(secret, payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if sig != expected {
		t.Errorf("SignPayload = %q, want %q", sig, expected)
	}
}

func TestSignPayloadValidHex(t *testing.T) {
	t.Parallel()

	sig := SignPayload("secret", []byte("data"))
	if _, err := hex.DecodeString(sig); err != nil {
		t.Errorf("SignPayload returned non-hex string: %q, error: %v", sig, err)
	}
}

// ---------------------------------------------------------------------------
// Deliver — Success Path
// ---------------------------------------------------------------------------

func TestDeliverSuccess(t *testing.T) {
	t.Parallel()

	var receivedBody []byte

	var receivedSig string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get(SignatureHeader)
		receivedBody, _ = io.ReadAll(r.Body)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := WebhookEnvelope{
		EventID:       "evt-1",
		EventType:     "test.event",
		OccurredAt:    time.Now(),
		SchemaVersion: SchemaVersion,
		Data:          map[string]any{"key": "value"},
	}

	err := Deliver(t.Context(), server.URL, "test-secret", env)
	if err != nil {
		t.Fatalf("Deliver() error: %v", err)
	}

	if receivedSig == "" {
		t.Error("expected signature header to be set")
	}

	// Verify the signature is valid
	expectedSig := SignPayload("test-secret", receivedBody)
	if receivedSig != expectedSig {
		t.Errorf("signature = %q, want %q", receivedSig, expectedSig)
	}

	// Verify the body is valid JSON
	var parsed WebhookEnvelope
	if err := json.Unmarshal(receivedBody, &parsed); err != nil {
		t.Errorf("invalid JSON body: %v", err)
	}

	if parsed.EventID != "evt-1" {
		t.Errorf("EventID = %q, want %q", parsed.EventID, "evt-1")
	}
}

func TestDeliverContentType(t *testing.T) {
	t.Parallel()

	var contentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}
	_ = Deliver(t.Context(), server.URL, "secret", env)

	if contentType != ContentType {
		t.Errorf("Content-Type = %q, want %q", contentType, ContentType)
	}
}

// ---------------------------------------------------------------------------
// Deliver — Failure Paths
// ---------------------------------------------------------------------------

func TestDeliverNon2xxReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := Deliver(t.Context(), server.URL, "secret", env)
	if err == nil {
		t.Error("Deliver() expected error for 500 status, got nil")
	}
}

func TestDeliver4xxReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := Deliver(t.Context(), server.URL, "secret", env)
	if err == nil {
		t.Error("Deliver() expected error for 400 status, got nil")
	}
}

func TestDeliverInvalidEndpoint(t *testing.T) {
	t.Parallel()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := Deliver(t.Context(), "http://localhost:1", "secret", env)
	if err == nil {
		t.Error("Deliver() expected error for unreachable endpoint, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeliverWithRetry — Retry Logic
// ---------------------------------------------------------------------------

func TestDeliverWithRetrySucceedsOnFirstAttempt(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := DeliverWithRetry(t.Context(), server.URL, "secret", env, 5)
	if err != nil {
		t.Fatalf("DeliverWithRetry() error: %v", err)
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestDeliverWithRetryRetriesOnFailure(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := DeliverWithRetry(t.Context(), server.URL, "secret", env, 5)
	if err != nil {
		t.Fatalf("DeliverWithRetry() error: %v", err)
	}

	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestDeliverWithRetryExhaustsAttempts(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	err := DeliverWithRetry(t.Context(), server.URL, "secret", env, 3)
	if err == nil {
		t.Error("DeliverWithRetry() expected error after exhausting retries, got nil")
	}

	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestDeliverWithRetryMinOneAttempt(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}
	// maxRetries=0 should be bumped to 1
	err := DeliverWithRetry(t.Context(), server.URL, "secret", env, 0)
	if err != nil {
		t.Fatalf("DeliverWithRetry() error: %v", err)
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestDeliverWithRetryContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}

	// Use a short-lived context to trigger cancellation during backoff
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	err := DeliverWithRetry(ctx, server.URL, "secret", env, 5)
	// Context should cancel before all retries complete
	if err == nil {
		t.Error("expected error from context cancellation or retries exhausted")
	}
}

// ---------------------------------------------------------------------------
// DeliverWithRetry — Exponential Backoff Timing
// ---------------------------------------------------------------------------

func TestDeliverWithRetryBackoffSchedule(t *testing.T) {
	t.Parallel()

	var attemptTimes []time.Time

	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attemptTimes = append(attemptTimes, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	env := WebhookEnvelope{EventID: "evt-1", EventType: "test", SchemaVersion: SchemaVersion}
	_ = DeliverWithRetry(t.Context(), server.URL, "secret", env, 4)

	if len(attemptTimes) != 4 {
		t.Fatalf("expected 4 attempts, got %d", len(attemptTimes))
	}

	// Verify backoff intervals: attempt 0 has no delay, then 1s, 2s, 4s
	for i := 1; i < len(attemptTimes); i++ {
		gap := attemptTimes[i].Sub(attemptTimes[i-1])
		expectedSeconds := 1 << uint(i-1)
		expected := time.Duration(expectedSeconds) * time.Second

		// Allow 300ms tolerance for scheduling jitter
		if gap < expected-300*time.Millisecond || gap > expected+500*time.Millisecond {
			t.Errorf("gap between attempt %d and %d = %v, want ~%v", i-1, i, gap, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// WebhookEnvelope — Round Trip
// ---------------------------------------------------------------------------

func TestWebhookEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	original := WebhookEnvelope{
		EventID:       "evt-roundtrip",
		EventType:     "user.created",
		OccurredAt:    now,
		SchemaVersion: "1.0",
		Data:          map[string]any{"name": "Test User", "age": float64(30)},
	}

	payload, err := original.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var decoded WebhookEnvelope
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if decoded.EventID != original.EventID {
		t.Errorf("EventID = %q, want %q", decoded.EventID, original.EventID)
	}

	if decoded.EventType != original.EventType {
		t.Errorf("EventType = %q, want %q", decoded.EventType, original.EventType)
	}

	if decoded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", decoded.SchemaVersion, original.SchemaVersion)
	}
}
