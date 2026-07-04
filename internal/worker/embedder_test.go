package worker

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// ContentEmbedder — Interface Compliance
// ---------------------------------------------------------------------------

// mockContentEmbedder implements ContentEmbedder for testing.
type mockContentEmbedder struct {
	callCount int
	lastText  string
	returnErr error
}

func (m *mockContentEmbedder) EmbedContent(_ context.Context, _, _ [16]byte, text string) error {
	m.callCount++
	m.lastText = text

	return m.returnErr
}

func TestContentEmbedderInterfaceCompliance(t *testing.T) {
	t.Parallel()

	var _ ContentEmbedder = (*mockContentEmbedder)(nil)
}

// ---------------------------------------------------------------------------
// RegisterContentEmbedder
// ---------------------------------------------------------------------------

func TestRegisterContentEmbedder(t *testing.T) {
	t.Parallel()
	// Reset global state.
	contentEmbedder = nil

	mock := &mockContentEmbedder{}
	RegisterContentEmbedder(mock)

	if contentEmbedder != mock {
		t.Error("RegisterContentEmbedder did not set the global variable")
	}

	// Reset.
	contentEmbedder = nil
}

func TestRegisterContentEmbedder_Nil(t *testing.T) {
	t.Parallel()

	contentEmbedder = nil
	RegisterContentEmbedder(nil)

	if contentEmbedder != nil {
		t.Error("expected nil after registering nil embedder")
	}
}
