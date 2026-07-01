package ai

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// ContentEmbedder — Construction
// ---------------------------------------------------------------------------

func TestNewContentEmbedder(t *testing.T) {
	d := NewDispatcher()
	ce := NewContentEmbedder(d, nil)
	if ce == nil {
		t.Fatal("NewContentEmbedder returned nil")
	}
	if ce.dispatcher != d {
		t.Error("dispatcher not set correctly")
	}
}

// ---------------------------------------------------------------------------
// ContentEmbedder — Nil Dispatcher Skip
// ---------------------------------------------------------------------------

func TestContentEmbedder_NilDispatcher(t *testing.T) {
	ce := NewContentEmbedder(nil, nil)
	err := ce.EmbedContent(context.Background(), [16]byte{1}, [16]byte{2}, "test")
	if err != nil {
		t.Fatalf("expected nil error with nil dispatcher (graceful skip), got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ContentEmbedder — Nil Writer
// ---------------------------------------------------------------------------

func TestContentEmbedder_NilWriter(t *testing.T) {
	d := NewDispatcher()
	p := &mockProvider{
		name: "embedder",
		sendFn: func(ctx context.Context, req Request) (Response, error) {
			return Response{
				ProviderRef: "test",
				Status:      "success",
				Raw:         map[string]any{"embedding": []float32{0.1}},
			}, nil
		},
	}
	d.RegisterProvider("embedding", p)

	ce := NewContentEmbedder(d, nil)
	err := ce.EmbedContent(context.Background(), [16]byte{1}, [16]byte{2}, "test")
	if err == nil {
		t.Fatal("expected error with nil writer, got nil")
	}
}
