package ama

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestAMAErrorSentinelCompat verifies the data-carrying error types
// still satisfy errors.Is against the original sentinels so existing
// callers that switch on the sentinels keep working.
func TestAMAErrorSentinelCompat(t *testing.T) {
	t.Parallel()

	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("scan id: %v", err)
	}

	active := &ActiveAMAError{SessionID: id}
	if !errors.Is(active, ErrActiveAMAExists) {
		t.Fatalf("ActiveAMAError must satisfy errors.Is(ErrActiveAMAExists)")
	}

	cooldown := &CooldownError{RetryAt: time.Now()}
	if !errors.Is(cooldown, ErrAMACooldown) {
		t.Fatalf("CooldownError must satisfy errors.Is(ErrAMACooldown)")
	}
}

func TestHandleCreateError_ActiveSession(t *testing.T) {
	t.Parallel()

	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("scan id: %v", err)
	}

	rr := httptest.NewRecorder()
	handleCreateError(rr, &ActiveAMAError{SessionID: id})

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Error struct {
			Code   string `json:"code"`
			Data   struct {
				SessionID string `json:"session_id"`
			} `json:"data"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Error.Code != "ama_active_exists" {
		t.Fatalf("expected code ama_active_exists, got %q", got.Error.Code)
	}

	if got.Error.Data.SessionID != id.String() {
		t.Fatalf("expected session_id %s, got %q", id.String(), got.Error.Data.SessionID)
	}
}

func TestHandleCreateError_Cooldown(t *testing.T) {
	t.Parallel()

	retryAt := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rr := httptest.NewRecorder()
	handleCreateError(rr, &CooldownError{RetryAt: retryAt})

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	var got struct {
		Error struct {
			Code   string `json:"code"`
			Data   struct {
				RetryAfter string `json:"retry_after"`
			} `json:"data"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got.Error.Code != "ama_cooldown" {
		t.Fatalf("expected code ama_cooldown, got %q", got.Error.Code)
	}

	want := retryAt.UTC().Format(time.RFC3339)
	if got.Error.Data.RetryAfter != want {
		t.Fatalf("expected retry_after %s, got %q", want, got.Error.Data.RetryAfter)
	}
}

// TestHandleCreateError_PlainSentinelsNoData verifies the plain
// sentinels (no data) still produce the canonical error envelope
// without a data field, preserving the pre-existing contract.
func TestHandleCreateError_PlainSentinelsNoData(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"active", ErrActiveAMAExists, http.StatusConflict},
		{"cooldown", ErrAMACooldown, http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			handleCreateError(rr, tc.err)

			if rr.Code != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, rr.Code, rr.Body.String())
			}

			var got map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}

			errBody, _ := got["error"].(map[string]any)
			if errBody == nil {
				t.Fatalf("expected error object, got %v", got)
			}

			if _, hasData := errBody["data"]; hasData {
				t.Fatalf("plain sentinel must not carry a data field, got %v", errBody)
			}
		})
	}
}
