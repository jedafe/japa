package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Provider Interface Compliance
// ---------------------------------------------------------------------------

func TestXiaomiProviderInterfaceCompliance(t *testing.T) {
	t.Parallel()

	var _ Provider = (*XiaomiProvider)(nil)
}

func TestXiaomiProviderName(t *testing.T) {
	t.Parallel()

	p := NewXiaomiProvider("sk-test", "")
	if got := p.Name(); got != XiaomiProviderName {
		t.Errorf("Name() = %q, want %q", got, XiaomiProviderName)
	}
}

// ---------------------------------------------------------------------------
// Constructor — NewXiaomiProvider
// ---------------------------------------------------------------------------

func TestNewXiaomiProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apiKey      string
		model       string
		wantBaseURL string
		wantModel   string
	}{
		{
			name:        "token plan key selects CN endpoint",
			apiKey:      "tp-abc123",
			model:       "",
			wantBaseURL: XiaomiBaseTokenPlanCN,
			wantModel:   XiaomiModelFlash,
		},
		{
			name:        "pay-as-you-go key selects PAYG endpoint",
			apiKey:      "sk-xyz789",
			model:       "",
			wantBaseURL: XiaomiBasePayg,
			wantModel:   XiaomiModelFlash,
		},
		{
			name:        "empty key defaults to PAYG endpoint",
			apiKey:      "",
			model:       "",
			wantBaseURL: XiaomiBasePayg,
			wantModel:   XiaomiModelFlash,
		},
		{
			name:        "explicit model is preserved",
			apiKey:      "sk-xyz789",
			model:       XiaomiModelPro,
			wantBaseURL: XiaomiBasePayg,
			wantModel:   XiaomiModelPro,
		},
		{
			name:        "token plan key with explicit model",
			apiKey:      "tp-abc123",
			model:       XiaomiModelOmni,
			wantBaseURL: XiaomiBaseTokenPlanCN,
			wantModel:   XiaomiModelOmni,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := NewXiaomiProvider(tt.apiKey, tt.model)
			if p == nil {
				t.Fatal("NewXiaomiProvider() returned nil")
			}

			if p.baseURL != tt.wantBaseURL {
				t.Errorf("baseURL = %q, want %q", p.baseURL, tt.wantBaseURL)
			}

			if p.model != tt.wantModel {
				t.Errorf("model = %q, want %q", p.model, tt.wantModel)
			}

			if p.apiKey != tt.apiKey {
				t.Errorf("apiKey = %q, want %q", p.apiKey, tt.apiKey)
			}

			if p.name != XiaomiProviderName {
				t.Errorf("name = %q, want %q", p.name, XiaomiProviderName)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// selectXiaomiBaseURL
// ---------------------------------------------------------------------------

func TestSelectXiaomiBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiKey  string
		wantURL string
	}{
		{"token plan key", "tp-abc123", XiaomiBaseTokenPlanCN},
		{"pay-as-you-go key", "sk-xyz789", XiaomiBasePayg},
		{"empty key", "", XiaomiBasePayg},
		{"unrelated prefix", "ak-something", XiaomiBasePayg},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := selectXiaomiBaseURL(tt.apiKey)
			if got != tt.wantURL {
				t.Errorf("selectXiaomiBaseURL(%q) = %q, want %q", tt.apiKey, got, tt.wantURL)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildMimoMessages
// ---------------------------------------------------------------------------

func TestBuildMimoMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  string
		payload map[string]any
		wantLen int
		wantTop string // role of the first message
	}{
		{
			name:    "prompt only",
			prompt:  "What is JapaConnect?",
			payload: map[string]any{},
			wantLen: 1,
			wantTop: "user",
		},
		{
			name:    "prompt with context",
			prompt:  "Explain visa requirements",
			payload: map[string]any{"context": "You are an immigration advisor"},
			wantLen: 2,
			wantTop: "system",
		},
		{
			name:    "prompt with empty context",
			prompt:  "Hello",
			payload: map[string]any{"context": ""},
			wantLen: 1,
			wantTop: "user",
		},
		{
			name:    "prompt with nil context value",
			prompt:  "Hello",
			payload: map[string]any{"context": nil},
			wantLen: 1,
			wantTop: "user",
		},
		{
			name:    "prompt with non-string context",
			prompt:  "Hello",
			payload: map[string]any{"context": 42},
			wantLen: 1,
			wantTop: "user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msgs := buildMimoMessages(tt.prompt, tt.payload)
			if len(msgs) != tt.wantLen {
				t.Fatalf("len(messages) = %d, want %d", len(msgs), tt.wantLen)
			}

			if msgs[0].Role != tt.wantTop {
				t.Errorf("messages[0].Role = %q, want %q", msgs[0].Role, tt.wantTop)
			}
			// Last message is always the user prompt.
			last := msgs[len(msgs)-1]
			if last.Role != "user" {
				t.Errorf("last message Role = %q, want %q", last.Role, "user")
			}

			if last.Content != tt.prompt {
				t.Errorf("last message Content = %q, want %q", last.Content, tt.prompt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty string", "", 10, ""},
		{"single char truncated", "ab", 1, "a..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Send — Success Flow
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_Success(t *testing.T) {
	t.Parallel()

	mockResp := mimoChatResponse{
		ID:    "chatcmpl-test-123",
		Model: "mimo-v2-flash",
		Choices: []mimoChoice{
			{
				Index:        0,
				FinishReason: "stop",
				Message:      mimoMessage{Role: "assistant", Content: "JapaConnect is a community platform."},
			},
		},
		Usage: mimoUsage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}

	var gotMethod, gotPath, gotAPIKey, gotContentType string

	var gotBody mimoChatRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("api-key")
		gotContentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test-key",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	resp, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "What is JapaConnect?"},
	})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}

	// Verify HTTP request details.
	t.Run("http request", func(t *testing.T) {
		t.Parallel()
		assertEqual(t, "method", gotMethod, http.MethodPost)
		assertEqual(t, "path", gotPath, "/chat/completions")
		assertEqual(t, "api-key header", gotAPIKey, "sk-test-key")
		assertEqual(t, "Content-Type", gotContentType, "application/json")
	})

	// Verify request body.
	t.Run("request body", func(t *testing.T) {
		t.Parallel()

		assertEqual(t, "model", gotBody.Model, XiaomiModelFlash)

		if len(gotBody.Messages) != 1 {
			t.Fatalf("messages len = %d, want 1", len(gotBody.Messages))
		}

		assertEqual(t, "message content", gotBody.Messages[0].Content, "What is JapaConnect?")

		if gotBody.Stream {
			t.Error("Stream should be false")
		}
	})

	// Verify response mapping.
	t.Run("response mapping", func(t *testing.T) {
		t.Parallel()
		assertEqual(t, "ProviderRef", resp.ProviderRef, "chatcmpl-test-123")
		assertEqual(t, "Status", resp.Status, "success")
		assertEqual(t, "Raw[id]", fmt.Sprint(resp.Raw["id"]), "chatcmpl-test-123")
		assertEqual(t, "Raw[model]", fmt.Sprint(resp.Raw["model"]), "mimo-v2-flash")
	})
}

// assertEqual is a test helper for simple string comparisons.
func assertEqual(t *testing.T, label, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %q, want %q", label, got, want)
	}
}

// ---------------------------------------------------------------------------
// Send — With System Context
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_WithSystemContext(t *testing.T) {
	t.Parallel()

	mockResp := mimoChatResponse{
		ID:    "chatcmpl-ctx-1",
		Model: "mimo-v2-flash",
		Choices: []mimoChoice{
			{Index: 0, FinishReason: "stop", Message: mimoMessage{Role: "assistant", Content: "response"}},
		},
		Usage: mimoUsage{TotalTokens: 10},
	}

	var gotMessages []mimoMessage

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var req mimoChatRequest
		_ = json.Unmarshal(body, &req)
		gotMessages = req.Messages

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload: map[string]any{
			"prompt":  "Explain visa requirements",
			"context": "You are an immigration advisor for Nigerians.",
		},
	})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}

	if len(gotMessages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(gotMessages))
	}

	if gotMessages[0].Role != "system" {
		t.Errorf("messages[0].Role = %q, want %q", gotMessages[0].Role, "system")
	}

	if gotMessages[0].Content != "You are an immigration advisor for Nigerians." {
		t.Errorf("messages[0].Content = %q", gotMessages[0].Content)
	}

	if gotMessages[1].Role != "user" {
		t.Errorf("messages[1].Role = %q, want %q", gotMessages[1].Role, "user")
	}
}

// ---------------------------------------------------------------------------
// Send — Error Cases
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_EmptyAPIKey(t *testing.T) {
	t.Parallel()

	p := NewXiaomiProvider("", "")

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for empty API key, got nil")
	}

	if !strings.Contains(err.Error(), "API key not configured") {
		t.Errorf("error = %q, want substring %q", err.Error(), "API key not configured")
	}
}

func TestXiaomiProviderSend_EmptyPrompt(t *testing.T) {
	t.Parallel()

	p := NewXiaomiProvider("sk-test", "")

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{},
	})
	if err == nil {
		t.Fatal("Send() expected error for empty prompt, got nil")
	}

	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "prompt is required")
	}
}

func TestXiaomiProviderSend_NilPayload(t *testing.T) {
	t.Parallel()

	p := NewXiaomiProvider("sk-test", "")

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  nil,
	})
	if err == nil {
		t.Fatal("Send() expected error for nil payload, got nil")
	}

	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("error = %q, want substring %q", err.Error(), "prompt is required")
	}
}

func TestXiaomiProviderSend_HTTPError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for HTTP 500, got nil")
	}

	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q, want substring %q", err.Error(), "status 500")
	}
}

func TestXiaomiProviderSend_MalformedResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for malformed response, got nil")
	}

	if !strings.Contains(err.Error(), "parsing response") {
		t.Errorf("error = %q, want substring %q", err.Error(), "parsing response")
	}
}

func TestXiaomiProviderSend_Timeout(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: client,
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected timeout error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Send — Auth and Rate Limit Errors
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_AuthError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-invalid",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for HTTP 401, got nil")
	}

	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("error = %q, want substring %q", err.Error(), "status 401")
	}
}

func TestXiaomiProviderSend_RateLimited(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for HTTP 429, got nil")
	}

	if !strings.Contains(err.Error(), "status 429") {
		t.Errorf("error = %q, want substring %q", err.Error(), "status 429")
	}
}

// ---------------------------------------------------------------------------
// Send — Context Cancellation
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_ContextCanceled(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	_, err := p.Send(ctx, Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("Send() expected error for canceled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// Send — Response with Empty Choices
// ---------------------------------------------------------------------------

func TestXiaomiProviderSend_EmptyChoices(t *testing.T) {
	t.Parallel()

	mockResp := mimoChatResponse{
		ID:      "chatcmpl-empty",
		Model:   "mimo-v2-flash",
		Choices: []mimoChoice{},
		Usage:   mimoUsage{TotalTokens: 5},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	p := &XiaomiProvider{
		name:       XiaomiProviderName,
		baseURL:    ts.URL,
		apiKey:     "sk-test",
		model:      XiaomiModelFlash,
		httpClient: ts.Client(),
	}

	resp, err := p.Send(t.Context(), Request{
		Category: "test",
		Payload:  map[string]any{"prompt": "hello"},
	})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}

	if resp.ProviderRef != "chatcmpl-empty" {
		t.Errorf("ProviderRef = %q, want %q", resp.ProviderRef, "chatcmpl-empty")
	}

	choices, ok := resp.Raw["choices"].([]mimoChoice)
	if !ok {
		t.Fatal("Raw[choices] not of expected type []mimoChoice")
	}

	if len(choices) != 0 {
		t.Errorf("len(choices) = %d, want 0", len(choices))
	}
}

// ---------------------------------------------------------------------------
// VerifyWebhookSignature — Unsupported (returns error)
// ---------------------------------------------------------------------------

func TestXiaomiProviderVerifyWebhookSignature(t *testing.T) {
	t.Parallel()

	p := NewXiaomiProvider("sk-test", "")

	err := p.VerifyWebhookSignature([]byte("payload"), "sig123")
	if err == nil {
		t.Error("VerifyWebhookSignature() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Constants — Model, Name, Base URLs
// ---------------------------------------------------------------------------

func TestXiaomiModelConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"XiaomiModelFlash", XiaomiModelFlash, "mimo-v2-flash"},
		{"XiaomiModelPro", XiaomiModelPro, "mimo-v2-pro"},
		{"XiaomiModelV25Pro", XiaomiModelV25Pro, "mimo-v2.5-pro"},
		{"XiaomiModelOmni", XiaomiModelOmni, "mimo-v2-omni"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("constant = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestXiaomiProviderNameConstant(t *testing.T) {
	t.Parallel()

	if XiaomiProviderName != "xiaomi" {
		t.Errorf("XiaomiProviderName = %q, want %q", XiaomiProviderName, "xiaomi")
	}
}

func TestXiaomiBaseURLConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"XiaomiBasePayg", XiaomiBasePayg, "https://api.xiaomimimo.com/v1"},
		{"XiaomiBaseTokenPlanCN", XiaomiBaseTokenPlanCN, "https://token-plan-cn.xiaomimimo.com/v1"},
		{"XiaomiBaseTokenPlanSGP", XiaomiBaseTokenPlanSGP, "https://token-plan-sgp.xiaomimimo.com/v1"},
		{"XiaomiBaseTokenPlanAMS", XiaomiBaseTokenPlanAMS, "https://token-plan-ams.xiaomimimo.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("constant = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
