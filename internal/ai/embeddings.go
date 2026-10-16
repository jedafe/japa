// Package ai — embeddings.go generates vector embeddings for content via the
// AI provider abstraction. The dispatcher handles provider fallback so a
// single provider failure never blocks embedding generation.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// DefaultEmbeddingModel is the embedding model used when no model is specified.
const DefaultEmbeddingModel = "text-embedding-3-small"

// EmbeddingDimension is the storage width of the vector columns
// (content_embeddings.embedding is vector(1536)). Providers returning
// shorter vectors (e.g. local Ollama models at 768) are zero-padded to this
// width — zero-padding preserves cosine distance exactly when applied to
// both indexed chunks and query vectors, and keeps every row in the shared
// column physically dimension-consistent for the HNSW index. Longer vectors
// are an error, not silently truncated.
const EmbeddingDimension = 1536

// EmbeddingPayload is the request payload for embedding generation.
// The Provider receives this as map[string]any via Request.Payload.
type EmbeddingPayload struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

// GenerateContentEmbedding generates a vector embedding for the given text
// using the AI dispatcher's "embedding" category. The returned slice is
// pgvector-compatible: a flat []float32 of dimension 1536.
func GenerateContentEmbedding(ctx context.Context, dispatcher *Dispatcher, text, model string) ([]float32, error) {
	if text == "" {
		return nil, errors.New("text is required for embedding generation")
	}

	if model == "" {
		model = DefaultEmbeddingModel
	}

	resp, err := dispatcher.Dispatch(ctx, "embedding", Request{
		Category: "embedding",
		Payload: map[string]any{
			"text":  text,
			"model": model,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	vec, err := parseEmbeddingResponse(resp)
	if err != nil {
		return nil, err
	}

	return padEmbedding(vec)
}

// padEmbedding normalizes a provider vector to EmbeddingDimension. Shorter
// vectors get zero-padded (cosine-invariant); longer vectors are an error —
// truncating would silently corrupt similarity math.
func padEmbedding(vec []float32) ([]float32, error) {
	switch {
	case len(vec) == EmbeddingDimension:
		return vec, nil
	case len(vec) > EmbeddingDimension:
		return nil, fmt.Errorf("embedding dimension %d exceeds storage width %d — configure a ≤%d-dim model", len(vec), EmbeddingDimension, EmbeddingDimension)
	default:
		padded := make([]float32, EmbeddingDimension)
		copy(padded, vec)
		return padded, nil
	}
}

// parseEmbeddingResponse extracts the embedding vector from a provider Response.
// The provider stores the embedding in Raw["embedding"] as either:
//   - []float32 (direct)
//   - []interface{} of float64 (JSON-deserialized)
//   - json.RawMessage (raw bytes to unmarshal)
func parseEmbeddingResponse(resp Response) ([]float32, error) {
	raw, ok := resp.Raw["embedding"]
	if !ok {
		return nil, errors.New("provider response missing embedding data")
	}

	switch v := raw.(type) {
	case []float32:
		return v, nil
	case []float64:
		out := make([]float32, len(v))
		for i, f := range v {
			out[i] = float32(f)
		}

		return out, nil
	case []interface{}:
		out := make([]float32, len(v))

		for i, elem := range v {
			switch n := elem.(type) {
			case float64:
				out[i] = float32(n)
			case float32:
				out[i] = n
			default:
				return nil, fmt.Errorf("unexpected embedding element type at index %d: %T", i, elem)
			}
		}

		return out, nil
	case json.RawMessage:
		var floats []float32
		if err := json.Unmarshal(v, &floats); err != nil {
			return nil, fmt.Errorf("unmarshaling embedding: %w", err)
		}

		return floats, nil
	default:
		return nil, fmt.Errorf("unexpected embedding response type: %T", raw)
	}
}
