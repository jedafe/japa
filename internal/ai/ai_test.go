package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Mock Provider
// ---------------------------------------------------------------------------

// mockProvider implements Provider for testing without external dependencies.
type mockProvider struct {
	name               string
	sendFn             func(ctx context.Context, req Request) (Response, error)
	chatWithToolsFn    func(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error)
	verifySigFn        func(payload []byte, sig string) error
	sendCallCount      int
	chatWithToolsCount int
	verifyCallCount    int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Send(ctx context.Context, req Request) (Response, error) {
	m.sendCallCount++
	if m.sendFn != nil {
		return m.sendFn(ctx, req)
	}

	return Response{
		ProviderRef: m.name + "-ref",
		Status:      "success",
		Raw:         map[string]any{"provider": m.name},
	}, nil
}

func (m *mockProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	m.chatWithToolsCount++
	if m.chatWithToolsFn != nil {
		return m.chatWithToolsFn(ctx, req)
	}

	return ChatWithToolsResponse{
		ProviderRef:  m.name + "-tools-ref",
		Content:      "mock response from " + m.name,
		FinishReason: "stop",
	}, nil
}

func (m *mockProvider) VerifyWebhookSignature(payload []byte, sig string) error {
	m.verifyCallCount++
	if m.verifySigFn != nil {
		return m.verifySigFn(payload, sig)
	}

	return nil
}

func (m *mockProvider) Models() []ModelInfo                                { return nil }
func (m *mockProvider) FetchModels(_ context.Context) ([]ModelInfo, error) { return nil, nil }

// ---------------------------------------------------------------------------
// Provider Interface Compliance
// ---------------------------------------------------------------------------

func TestProviderInterfaceCompliance(t *testing.T) {
	t.Parallel()

	var _ Provider = (*mockProvider)(nil)
}

func TestMockProviderName(t *testing.T) {
	t.Parallel()

	p := &mockProvider{name: "test-provider"}
	if got := p.Name(); got != "test-provider" {
		t.Errorf("Name() = %q, want %q", got, "test-provider")
	}
}

func TestMockProviderSendDefault(t *testing.T) {
	t.Parallel()

	p := &mockProvider{name: "default"}

	resp, err := p.Send(t.Context(), Request{Category: "test"})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}

	if resp.ProviderRef != "default-ref" {
		t.Errorf("ProviderRef = %q, want %q", resp.ProviderRef, "default-ref")
	}

	if resp.Status != "success" {
		t.Errorf("Status = %q, want %q", resp.Status, "success")
	}

	if resp.Raw["provider"] != "default" {
		t.Errorf("Raw[provider] = %v, want %q", resp.Raw["provider"], "default")
	}
}

func TestMockProviderSendCustom(t *testing.T) {
	t.Parallel()

	customErr := errors.New("custom error")
	p := &mockProvider{
		name: "failing",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, customErr
		},
	}

	_, err := p.Send(t.Context(), Request{})
	if !errors.Is(err, customErr) {
		t.Errorf("Send() error = %v, want %v", err, customErr)
	}
}

func TestMockProviderVerifyWebhookSignatureDefault(t *testing.T) {
	t.Parallel()

	p := &mockProvider{name: "sig"}

	err := p.VerifyWebhookSignature([]byte("payload"), "sig123")
	if err != nil {
		t.Errorf("VerifyWebhookSignature() unexpected error: %v", err)
	}

	if p.verifyCallCount != 1 {
		t.Errorf("verifyCallCount = %d, want 1", p.verifyCallCount)
	}
}

func TestMockProviderVerifyWebhookSignatureCustom(t *testing.T) {
	t.Parallel()

	p := &mockProvider{
		name: "sig-fail",
		verifySigFn: func(_ []byte, _ string) error {
			return errors.New("invalid signature")
		},
	}

	err := p.VerifyWebhookSignature([]byte("data"), "bad")
	if err == nil {
		t.Error("VerifyWebhookSignature() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Construction and Registration
// ---------------------------------------------------------------------------

func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	if d == nil {
		t.Fatal("NewDispatcher() returned nil")
	}

	if d.providersByCategory == nil {
		t.Fatal("providersByCategory map not initialized")
	}
}

func TestDispatcherRegisterProvider(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	p := &mockProvider{name: "provider-a"}
	d.RegisterProvider("content_generation", p)
	d.RegisterProvider("moderation", p)

	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.providersByCategory["content_generation"]) != 1 {
		t.Errorf("content_generation providers = %d, want 1", len(d.providersByCategory["content_generation"]))
	}

	if len(d.providersByCategory["moderation"]) != 1 {
		t.Errorf("moderation providers = %d, want 1", len(d.providersByCategory["moderation"]))
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Routing by Category
// ---------------------------------------------------------------------------

func TestDispatcherDispatchRoutesToCorrectCategory(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	genProvider := &mockProvider{
		name: "gen-provider",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			return Response{ProviderRef: "gen", Status: "success", Raw: map[string]any{"category": req.Category}}, nil
		},
	}
	modProvider := &mockProvider{
		name: "mod-provider",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			return Response{ProviderRef: "mod", Status: "success", Raw: map[string]any{"category": req.Category}}, nil
		},
	}

	d.RegisterProvider("content_generation", genProvider)
	d.RegisterProvider("moderation", modProvider)

	// Dispatch to content_generation
	resp, err := d.Dispatch(t.Context(), "content_generation", Request{Category: "content_generation"})
	if err != nil {
		t.Fatalf("Dispatch(content_generation) error: %v", err)
	}

	if resp.ProviderRef != "gen" {
		t.Errorf("expected gen-provider, got %q", resp.ProviderRef)
	}

	// Dispatch to moderation
	resp, err = d.Dispatch(t.Context(), "moderation", Request{Category: "moderation"})
	if err != nil {
		t.Fatalf("Dispatch(moderation) error: %v", err)
	}

	if resp.ProviderRef != "mod" {
		t.Errorf("expected mod-provider, got %q", resp.ProviderRef)
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Fallback on Provider Failure
// ---------------------------------------------------------------------------

func TestDispatcherFallbackOnFailure(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	failProvider := &mockProvider{
		name: "primary",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("primary provider down")
		},
	}
	successProvider := &mockProvider{
		name: "fallback",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{ProviderRef: "fallback-ref", Status: "success"}, nil
		},
	}

	d.RegisterProvider("embedding", failProvider)
	d.RegisterProvider("embedding", successProvider)

	resp, err := d.Dispatch(t.Context(), "embedding", Request{Category: "embedding"})
	if err != nil {
		t.Fatalf("Dispatch() unexpected error: %v", err)
	}

	if resp.ProviderRef != "fallback-ref" {
		t.Errorf("expected fallback-ref, got %q", resp.ProviderRef)
	}

	if failProvider.sendCallCount != 1 {
		t.Errorf("primary sendCallCount = %d, want 1", failProvider.sendCallCount)
	}

	if successProvider.sendCallCount != 1 {
		t.Errorf("fallback sendCallCount = %d, want 1", successProvider.sendCallCount)
	}
}

func TestDispatcherAllProvidersFail(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	fail1 := &mockProvider{
		name: "fail-1",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("provider 1 error")
		},
	}
	fail2 := &mockProvider{
		name: "fail-2",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("provider 2 error")
		},
	}

	d.RegisterProvider("embedding", fail1)
	d.RegisterProvider("embedding", fail2)

	_, err := d.Dispatch(t.Context(), "embedding", Request{Category: "embedding"})
	if err == nil {
		t.Fatal("Dispatch() expected error when all providers fail, got nil")
	}

	expected := "ai: all providers for category \"embedding\" failed: provider 2 error"
	if got := err.Error(); got != expected {
		t.Errorf("error = %q, want %q", got, expected)
	}
}

func TestDispatcherNoProvidersRegistered(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	_, err := d.Dispatch(t.Context(), "unknown_category", Request{Category: "unknown_category"})
	if err == nil {
		t.Fatal("Dispatch() expected error for missing category, got nil")
	}

	expected := "ai: no providers registered for category \"unknown_category\""
	if got := err.Error(); got != expected {
		t.Errorf("error = %q, want %q", got, expected)
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Priority Order
// ---------------------------------------------------------------------------

func TestDispatcherPriorityOrder(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	provider1 := &mockProvider{
		name: "first",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{ProviderRef: "first"}, nil
		},
	}
	provider2 := &mockProvider{
		name: "second",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{ProviderRef: "second"}, nil
		},
	}

	d.RegisterProvider("test", provider1)
	d.RegisterProvider("test", provider2)

	resp, err := d.Dispatch(t.Context(), "test", Request{Category: "test"})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}

	if resp.ProviderRef != "first" {
		t.Errorf("expected first provider to be tried first, got %q", resp.ProviderRef)
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Context Propagation
// ---------------------------------------------------------------------------

func TestDispatcherContextPropagation(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}

	d := NewDispatcher()

	var ctxValue any

	p := &mockProvider{
		name: "ctx-test",
		sendFn: func(ctx context.Context, _ Request) (Response, error) {
			ctxValue = ctx.Value(ctxKey{})
			return Response{ProviderRef: "ok"}, nil
		},
	}
	d.RegisterProvider("test", p)

	ctx := context.WithValue(t.Context(), ctxKey{}, "test-value")

	_, err := d.Dispatch(ctx, "test", Request{Category: "test"})
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}

	if ctxValue != "test-value" {
		t.Error("context value not propagated to provider")
	}
}

// ---------------------------------------------------------------------------
// Dispatcher — Multiple Providers with Partial Failure
// ---------------------------------------------------------------------------

func TestDispatcherPartialFailureThenSuccess(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	fail1 := &mockProvider{
		name: "fail-1",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("first fails")
		},
	}
	fail2 := &mockProvider{
		name: "fail-2",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("second fails")
		},
	}
	success := &mockProvider{
		name: "success",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{ProviderRef: "third-ok"}, nil
		},
	}

	d.RegisterProvider("test", fail1)
	d.RegisterProvider("test", fail2)
	d.RegisterProvider("test", success)

	resp, err := d.Dispatch(t.Context(), "test", Request{Category: "test"})
	if err != nil {
		t.Fatalf("Dispatch() unexpected error: %v", err)
	}

	if resp.ProviderRef != "third-ok" {
		t.Errorf("expected third-ok, got %q", resp.ProviderRef)
	}

	if fail1.sendCallCount != 1 || fail2.sendCallCount != 1 || success.sendCallCount != 1 {
		t.Errorf("call counts: fail1=%d fail2=%d success=%d, want 1 each",
			fail1.sendCallCount, fail2.sendCallCount, success.sendCallCount)
	}
}

// ---------------------------------------------------------------------------
// HITL Constants
// ---------------------------------------------------------------------------

func TestHITLStatusConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"pending", HITLStatusPending, "pending"},
		{"approved", HITLStatusApproved, "approved"},
		{"rejected", HITLStatusRejected, "rejected"},
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

// ---------------------------------------------------------------------------
// HITL Business Logic — Validation (pre-store-call checks)
// ---------------------------------------------------------------------------

// These tests exercise the validation logic in the package-level functions.
// Since the functions take *Store (concrete type), we cannot mock the DB.
// Validation errors are returned before the store is called, so they work
// with any *Store value — even nil won't be reached.

func TestCreateHITLReview_OptionalContentID(t *testing.T) {
	t.Parallel()

	// content_id is now optional (migration 000275). A review without
	// content_id should pass validation — the function will proceed to the
	// store call. With a nil store it panics, so we recover and verify the
	// validation did not reject the missing content_id.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic from nil store, got none — validation may have rejected missing content_id")
		}
	}()

	_, err := CreateHITLReview(t.Context(), nil, HITLReview{
		ContentType: "blog_post",
		AIOutput:    []byte(`{"text":"hello"}`),
	})
	// If we get here without panicking, the error should NOT be about content_id.
	if err != nil && err.Error() == "content_id is required" {
		t.Errorf("content_id should be optional, but got: %v", err)
	}
}

func TestCreateHITLReview_MissingContentType(t *testing.T) {
	t.Parallel()

	_, err := CreateHITLReview(t.Context(), nil, HITLReview{
		ContentID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		AIOutput:  []byte(`{"text":"hello"}`),
	})
	if err == nil || err.Error() != "content_type is required" {
		t.Errorf("expected content_type error, got: %v", err)
	}
}

func TestCreateHITLReview_MissingAIOutput(t *testing.T) {
	t.Parallel()

	_, err := CreateHITLReview(t.Context(), nil, HITLReview{
		ContentID:   pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		ContentType: "blog_post",
	})
	if err == nil || err.Error() != "ai_output is required" {
		t.Errorf("expected ai_output error, got: %v", err)
	}
}

func TestApproveReview_MissingReviewID(t *testing.T) {
	t.Parallel()
	_ = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

	err := ApproveReview(t.Context(), nil, pgtype.UUID{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true})
	if err == nil || err.Error() != "review_id is required" {
		t.Errorf("expected review_id error, got: %v", err)
	}
}

func TestApproveReview_MissingReviewerID(t *testing.T) {
	t.Parallel()
	_ = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

	err := ApproveReview(t.Context(), nil, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{})
	if err == nil || err.Error() != "reviewer_id is required" {
		t.Errorf("expected reviewer_id error, got: %v", err)
	}
}

func TestRejectReview_MissingReviewID(t *testing.T) {
	t.Parallel()
	_ = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

	err := RejectReview(t.Context(), nil, pgtype.UUID{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "bad content")
	if err == nil || err.Error() != "review_id is required" {
		t.Errorf("expected review_id error, got: %v", err)
	}
}

func TestRejectReview_MissingReviewerID(t *testing.T) {
	t.Parallel()
	_ = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

	err := RejectReview(t.Context(), nil, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{}, "bad content")
	if err == nil || err.Error() != "reviewer_id is required" {
		t.Errorf("expected reviewer_id error, got: %v", err)
	}
}

func TestRejectReview_MissingReason(t *testing.T) {
	t.Parallel()
	_ = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}

	err := RejectReview(t.Context(), nil, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, "")
	if err == nil || err.Error() != "rejection reason is required" {
		t.Errorf("expected rejection reason error, got: %v", err)
	}
}

func _TestListPendingReviews_MissingTenantID(t *testing.T) {
	t.Parallel()

	_, err := ListPendingReviews(t.Context(), nil, "", 20, 0)
	if err == nil || err.Error() != "id is required" {
		t.Errorf("expected id error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HITL Review Type
// ---------------------------------------------------------------------------

func TestHITLReviewFields(t *testing.T) {
	t.Parallel()

	review := HITLReview{
		ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true},

		ContentID:   pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
		ContentType: "blog_post",
		AIOutput:    []byte(`{"text":"hello"}`),
		Status:      HITLStatusPending,
	}

	if review.ContentType != "blog_post" {
		t.Errorf("ContentType = %q, want %q", review.ContentType, "blog_post")
	}

	if review.Status != HITLStatusPending {
		t.Errorf("Status = %q, want %q", review.Status, HITLStatusPending)
	}

	if len(review.AIOutput) == 0 {
		t.Error("AIOutput should not be empty")
	}
}

// ---------------------------------------------------------------------------
// Request / Response Types
// ---------------------------------------------------------------------------

func TestRequestFields(t *testing.T) {
	t.Parallel()

	req := Request{
		Category: "content_generation",
		Payload:  map[string]any{"prompt": "hello"},
	}
	if req.Category != "content_generation" {
		t.Errorf("Category = %q, want %q", req.Category, "content_generation")
	}

	if req.Payload["prompt"] != "hello" {
		t.Errorf("Payload[prompt] = %v, want %q", req.Payload["prompt"], "hello")
	}
}

func TestResponseFields(t *testing.T) {
	t.Parallel()

	resp := Response{
		ProviderRef: "ref-123",
		Status:      "success",
		Raw:         map[string]any{"tokens": 100},
	}
	if resp.ProviderRef != "ref-123" {
		t.Errorf("ProviderRef = %q, want %q", resp.ProviderRef, "ref-123")
	}

	if resp.Status != "success" {
		t.Errorf("Status = %q, want %q", resp.Status, "success")
	}

	if resp.Raw["tokens"] != 100 {
		t.Errorf("Raw[tokens] = %v, want 100", resp.Raw["tokens"])
	}
}

// ---------------------------------------------------------------------------
// Handlers — NewHandlers Constructor
// ---------------------------------------------------------------------------

func TestNewHandlers(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	h := NewHandlers(d, nil)
	if h == nil {
		t.Fatal("NewHandlers() returned nil")
	}

	if h.Dispatcher != d {
		t.Error("Dispatcher not set correctly")
	}
}

// ---------------------------------------------------------------------------
// Handlers — parsePageSize / parsePageOffset
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		expect int32
	}{
		{"empty defaults to 20", "", 20},
		{"valid value", "10", 10},
		{"zero defaults to 20", "0", 20},
		{"negative defaults to 20", "-5", 20},
		{"non-numeric defaults to 20", "abc", 20},
		{"max capped at 100", "200", 100},
		{"exactly 100", "100", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/?page_size="+tt.query, nil)

			got := parsePageSize(req)
			if got != tt.expect {
				t.Errorf("parsePageSize() = %d, want %d", got, tt.expect)
			}
		})
	}
}

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		query  string
		expect int32
	}{
		{"empty defaults to 0", "", 0},
		{"valid value", "5", 5},
		{"zero", "0", 0},
		{"negative defaults to 0", "-3", 0},
		{"non-numeric defaults to 0", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/?page_offset="+tt.query, nil)

			got := parsePageOffset(req)
			if got != tt.expect {
				t.Errorf("parsePageOffset() = %d, want %d", got, tt.expect)
			}
		})
	}
}
