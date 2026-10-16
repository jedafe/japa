package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// GenerateContentEmbedding — Validation
// ---------------------------------------------------------------------------

func TestGenerateContentEmbedding_EmptyText(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	_, err := GenerateContentEmbedding(t.Context(), d, "", "")
	if err == nil {
		t.Fatal("expected error for empty text, got nil")
	}

	if err.Error() != "text is required for embedding generation" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateContentEmbedding_NoProviders(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	_, err := GenerateContentEmbedding(t.Context(), d, "hello world", "")
	if err == nil {
		t.Fatal("expected error when no providers registered, got nil")
	}
}

// ---------------------------------------------------------------------------
// GenerateContentEmbedding — Success
// ---------------------------------------------------------------------------

func TestGenerateContentEmbedding_Success(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	expected := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	p := &mockProvider{
		name: "test-embedder",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			if req.Category != "embedding" {
				t.Errorf("expected category 'embedding', got %q", req.Category)
			}
			if req.Payload["model"] != DefaultEmbeddingModel {
				t.Errorf("expected default model, got %v", req.Payload["model"])
			}
			return Response{
				ProviderRef: "test-ref",
				Status:      "success",
				Raw:         map[string]any{"embedding": expected},
			}, nil
		},
	}
	d.RegisterProvider("embedding", p)

	got, err := GenerateContentEmbedding(t.Context(), d, "test content", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provider dims shorter than the storage width are zero-padded to
	// EmbeddingDimension — the documented contract of this function.
	if len(got) != EmbeddingDimension {
		t.Fatalf("expected %d dimensions, got %d", EmbeddingDimension, len(got))
	}

	for i, v := range expected {
		if got[i] != v {
			t.Errorf("dimension %d: got %v, want %v", i, got[i], v)
		}
	}
}

func TestGenerateContentEmbedding_CustomModel(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	var capturedModel string

	p := &mockProvider{
		name: "test-embedder",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			capturedModel, _ = req.Payload["model"].(string)
			return Response{
				ProviderRef: "test-ref",
				Status:      "success",
				Raw:         map[string]any{"embedding": []float32{0.1}},
			}, nil
		},
	}
	d.RegisterProvider("embedding", p)

	_, err := GenerateContentEmbedding(t.Context(), d, "test", "text-embedding-ada-002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedModel != "text-embedding-ada-002" {
		t.Errorf("expected model 'text-embedding-ada-002', got %q", capturedModel)
	}
}

// ---------------------------------------------------------------------------
// GenerateContentEmbedding — Provider Failure
// ---------------------------------------------------------------------------

func TestGenerateContentEmbedding_ProviderError(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	p := &mockProvider{
		name: "failing",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("provider rate limited")
		},
	}
	d.RegisterProvider("embedding", p)

	_, err := GenerateContentEmbedding(t.Context(), d, "test", "")
	if err == nil {
		t.Fatal("expected error from failing provider, got nil")
	}
}

// ---------------------------------------------------------------------------
// GenerateContentEmbedding — Fallback
// ---------------------------------------------------------------------------

func TestGenerateContentEmbedding_Fallback(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	fail := &mockProvider{
		name: "primary",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{}, errors.New("primary down")
		},
	}
	success := &mockProvider{
		name: "fallback",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{
				ProviderRef: "fallback-ref",
				Status:      "success",
				Raw:         map[string]any{"embedding": []float32{1.0}},
			}, nil
		},
	}

	d.RegisterProvider("embedding", fail)
	d.RegisterProvider("embedding", success)

	got, err := GenerateContentEmbedding(t.Context(), d, "test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != EmbeddingDimension || got[0] != 1.0 {
		t.Errorf("expected 1536-dim vector with got[0]==1.0, got %v", got[:3])
	}
}

// ---------------------------------------------------------------------------
// parseEmbeddingResponse — All Type Variants
// ---------------------------------------------------------------------------

func TestParseEmbeddingResponse_Float32(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": []float32{0.1, 0.2}}}

	got, err := parseEmbeddingResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 || got[0] != 0.1 || got[1] != 0.2 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseEmbeddingResponse_Float64(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": []float64{0.1, 0.2}}}

	got, err := parseEmbeddingResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}

	if got[0] != 0.1 || got[1] != 0.2 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseEmbeddingResponse_InterfaceSlice(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": []interface{}{0.5, 0.6, 0.7}}}

	got, err := parseEmbeddingResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(got))
	}

	if got[0] != 0.5 || got[1] != 0.6 || got[2] != 0.7 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseEmbeddingResponse_JSONRawMessage(t *testing.T) {
	t.Parallel()

	data, _ := json.Marshal([]float32{0.3, 0.4})
	resp := Response{Raw: map[string]any{"embedding": json.RawMessage(data)}}

	got, err := parseEmbeddingResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 2 || got[0] != 0.3 || got[1] != 0.4 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseEmbeddingResponse_MissingKey(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{}}

	_, err := parseEmbeddingResponse(resp)
	if err == nil {
		t.Fatal("expected error for missing embedding key, got nil")
	}
}

func TestParseEmbeddingResponse_UnexpectedType(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": "not-a-vector"}}

	_, err := parseEmbeddingResponse(resp)
	if err == nil {
		t.Fatal("expected error for unexpected type, got nil")
	}
}

func TestParseEmbeddingResponse_BadInterfaceElement(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": []interface{}{"bad"}}}

	_, err := parseEmbeddingResponse(resp)
	if err == nil {
		t.Fatal("expected error for bad interface element, got nil")
	}
}

func TestParseEmbeddingResponse_BadJSONRawMessage(t *testing.T) {
	t.Parallel()

	resp := Response{Raw: map[string]any{"embedding": json.RawMessage([]byte("not json"))}}

	_, err := parseEmbeddingResponse(resp)
	if err == nil {
		t.Fatal("expected error for bad JSON raw message, got nil")
	}
}
