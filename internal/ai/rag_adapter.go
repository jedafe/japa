// Package ai — rag_adapter.go adapts the AI Dispatcher to the
// rag.EmbeddingGenerator interface. The RAG service uses this to generate
// query embeddings at retrieval time and chunk embeddings at index time
// without depending on the ai package directly.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//
//	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package ai

import "context"

// RAGEmbedder adapts a Dispatcher to the rag.EmbeddingGenerator interface.
// It delegates to GenerateContentEmbedding, which calls the "embedding"
// category provider via the dispatcher's fallback chain.
type RAGEmbedder struct {
	dispatcher *Dispatcher
}

// NewRAGEmbedder creates a rag.EmbeddingGenerator backed by the AI dispatcher.
func NewRAGEmbedder(dispatcher *Dispatcher) *RAGEmbedder {
	return &RAGEmbedder{dispatcher: dispatcher}
}

// GenerateEmbedding generates a vector embedding for the given text via the
// AI gateway's "embedding" category.
func (r *RAGEmbedder) GenerateEmbedding(ctx context.Context, text, model string) ([]float32, error) {
	return GenerateContentEmbedding(ctx, r.dispatcher, text, model)
}

// Configured reports whether the dispatcher has a provider registered for
// the "embedding" category. The embedder wrapper is always non-nil once
// wired, so rag.Service.EmbedderConfigured type-asserts this to answer
// "is search actually available" honestly (§69 / agent data API Phase 2).
func (r *RAGEmbedder) Configured() bool {
	return r.dispatcher != nil && r.dispatcher.HasCategory("embedding")
}
