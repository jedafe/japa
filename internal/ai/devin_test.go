// Package ai — devin_test.go verifies the Devin provider against a mock
// Devin v3 API server.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md
package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDevinProvider_Send_StructuredOutput(t *testing.T) {
	pollCount := new(atomic.Int32)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/organizations/org-test/sessions":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for create: %s", r.Method)
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-123",
				"status":     "new",
				"url":        "https://app.devin.ai/sessions/sid-123",
			})
		case "/v3/organizations/org-test/sessions/sid-123":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method for poll: %s", r.Method)
			}

			c := pollCount.Add(1)
			if c < 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"session_id": "sid-123",
					"status":     "running",
					"url":        "https://app.devin.ai/sessions/sid-123",
				})

				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-123",
				"status":     "exit",
				"url":        "https://app.devin.ai/sessions/sid-123",
				"structured_output": map[string]any{
					"title":     "Test Title",
					"body_html": "<p>Test body</p>",
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := NewDevinProvider("devin-test", "cog_test", server.URL+"/v3/organizations/org-test", "")
	p.httpClient.Timeout = 5 * time.Second
	p.pollInterval = 10 * time.Millisecond
	p.pollTimeout = 1 * time.Second

	resp, err := p.Send(t.Context(), Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt":          "write a blog post",
			"response_format": map[string]any{"type": "json_object"},
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.ProviderRef != "sid-123" {
		t.Errorf("ProviderRef = %q, want sid-123", resp.ProviderRef)
	}

	if resp.Status != "success" {
		t.Errorf("Status = %q, want success", resp.Status)
	}

	got, _ := resp.Raw["generated_text"].(string)

	var parsed map[string]any

	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("generated_text is not valid JSON: %v", err)
	}

	if parsed["title"] != "Test Title" {
		t.Errorf("title = %v, want %q", parsed["title"], "Test Title")
	}

	if parsed["body_html"] != "<p>Test body</p>" {
		t.Errorf("body_html = %v, want %q", parsed["body_html"], "<p>Test body</p>")
	}

	if pollCount.Load() < 2 {
		t.Errorf("poll count = %d, want at least 2", pollCount.Load())
	}
}

func TestDevinProvider_Send_MessagesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/organizations/org-test/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-456",
				"status":     "new",
				"url":        "https://app.devin.ai/sessions/sid-456",
			})
		case "/v3/organizations/org-test/sessions/sid-456":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-456",
				"status":     "exit",
				"url":        "https://app.devin.ai/sessions/sid-456",
			})
		case "/v3/organizations/org-test/sessions/sid-456/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"event_id": "e1", "source": "user", "message": "hello", "created_at": 1},
					{"event_id": "e2", "source": "devin", "message": "Here is your answer.", "created_at": 2},
				},
				"has_next_page": false,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := NewDevinProvider("devin-test", "cog_test", server.URL+"/v3/organizations/org-test", "")
	p.httpClient.Timeout = 5 * time.Second
	p.pollInterval = 10 * time.Millisecond
	p.pollTimeout = 1 * time.Second

	resp, err := p.Send(t.Context(), Request{
		Category: "chat",
		Payload: map[string]any{
			"prompt": "hello",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("Status = %q, want success", resp.Status)
	}

	if got, _ := resp.Raw["reply"].(string); got != "Here is your answer." {
		t.Errorf("reply = %q, want %q", got, "Here is your answer.")
	}

	if got, _ := resp.Raw["generated_text"].(string); got != "Here is your answer." {
		t.Errorf("generated_text = %q, want %q", got, "Here is your answer.")
	}
}

func TestDevinProvider_Send_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/organizations/org-test/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-err",
				"status":     "new",
				"url":        "https://app.devin.ai/sessions/sid-err",
			})
		case "/v3/organizations/org-test/sessions/sid-err":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id":    "sid-err",
				"status":        "error",
				"status_detail": "usage_limit_exceeded",
				"url":           "https://app.devin.ai/sessions/sid-err",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	p := NewDevinProvider("devin-test", "cog_test", server.URL+"/v3/organizations/org-test", "")
	p.httpClient.Timeout = 5 * time.Second
	p.pollInterval = 10 * time.Millisecond
	p.pollTimeout = 1 * time.Second

	_, err := p.Send(t.Context(), Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": "write a blog post",
		},
	})
	if err == nil {
		t.Fatal("expected error for failed session")
	}

	if !strings.Contains(err.Error(), "usage_limit_exceeded") {
		t.Errorf("error = %q, want usage_limit_exceeded", err.Error())
	}
}

func TestDevinProvider_Send_ContextTimeout(t *testing.T) {
	callCount := new(atomic.Int32)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)

		switch r.URL.Path {
		case "/v3/organizations/org-test/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-timeout",
				"status":     "new",
				"url":        "https://app.devin.ai/sessions/sid-timeout",
			})
		case "/v3/organizations/org-test/sessions/sid-timeout":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "sid-timeout",
				"status":     "running",
				"url":        "https://app.devin.ai/sessions/sid-timeout",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewDevinProvider("devin-test", "cog_test", server.URL+"/v3/organizations/org-test", "")
	p.httpClient.Timeout = 5 * time.Second
	p.pollInterval = 500 * time.Millisecond
	p.pollTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	// Poll interval is 500ms and context deadline is 100ms, so the context
	// should cancel before the first poll tick.
	_, err := p.Send(ctx, Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": "write a blog post",
		},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "did not complete") {
		t.Errorf("error = %q, want timeout message", err.Error())
	}

	if callCount.Load() < 1 {
		t.Errorf("create was not called, callCount = %d", callCount.Load())
	}
}

func TestDevinProvider_Models_ReturnsAllModes(t *testing.T) {
	p := NewDevinProvider("devin-test", "cog_test", "", "fast")

	models := p.Models()
	if len(models) != 5 {
		t.Fatalf("Models() returned %d modes, want 5", len(models))
	}

	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}

	for _, want := range []string{"normal", "fast", "lite", "ultra", "fusion"} {
		if !ids[want] {
			t.Errorf("missing mode %q", want)
		}
	}
}

func TestDevinProvider_devinMode(t *testing.T) {
	tests := []struct {
		name       string
		configMode string
		payload    map[string]any
		want       string
	}{
		{"empty defaults to normal", "", map[string]any{}, "normal"},
		{"config mode used", "fast", map[string]any{}, "fast"},
		{"payload overrides config", "fast", map[string]any{"model": "lite"}, "lite"},
		{"invalid payload falls back", "fast", map[string]any{"model": "not-a-mode"}, "fast"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewDevinProvider("devin-test", "cog_test", "", tc.configMode)

			got := p.devinMode(Request{Payload: tc.payload})
			if got != tc.want {
				t.Errorf("devinMode() = %q, want %q", got, tc.want)
			}
		})
	}
}
