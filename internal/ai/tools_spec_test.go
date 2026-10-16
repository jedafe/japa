package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSpecSearchHandler_MissingQuery(t *testing.T) {
	t.Parallel()

	h := SpecSearchHandler(&ToolDeps{RAG: &stubRAGService{}})
	tr, err := h(context.Background(), json.RawMessage(`{"query":""}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("expected error result, got %+v", tr)
	}
}

func TestSpecSearchHandler_FlagOff(t *testing.T) {
	t.Parallel()

	h := SpecSearchHandler(&ToolDeps{RAG: &stubRAGService{err: ErrSpecCorpusDisabled}})
	tr, err := h(context.Background(), json.RawMessage(`{"query":"wallet ledger"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("expected error result, got %+v", tr)
	}
}

func TestSpecSearchHandler_Success(t *testing.T) {
	t.Parallel()

	stub := &stubRAGService{chunks: []RAGChunk{
		{EntityID: "x", EntityType: "spec_doc", ChunkText: "[spec: sections/44.md]\n\nbody", Similarity: 0.9, Tier: "canonical", Title: "Docs", Slug: "sections/44.md"},
	}}

	h := SpecSearchHandler(&ToolDeps{RAG: stub})
	tr, err := h(context.Background(), json.RawMessage(`{"query":"docs system","path_prefix":"sections/","limit":4}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("expected success result, got %+v", tr)
	}
}
