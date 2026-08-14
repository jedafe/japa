// Package rag — chunk.go splits plain text into overlapping chunks for
// embedding. Each chunk is at most maxTokens tokens (approximated by word
// count with a 1.3x safety factor), with a configurable overlap between
// consecutive chunks to preserve context across boundaries.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
//
//	japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md
package rag

import (
	"strings"
)

// ChunkText splits plain text into overlapping chunks. maxTokens is the
// approximate maximum token count per chunk; overlapTokens is the number of
// tokens to overlap between consecutive chunks. The token count is
// approximated as wordCount / 0.75 (roughly 4 chars per token, 3 chars per
// word → 1.33 tokens per word). This avoids pulling in a tokenizer
// dependency; the exact count is not critical for chunking quality.
func ChunkText(text string, maxTokens, overlapTokens int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	if maxTokens <= 0 {
		return []string{text}
	}

	// Approximate words-per-chunk and overlap in words.
	maxWords := int(float64(maxTokens) / 1.3)
	overlapWords := int(float64(overlapTokens) / 1.3)
	if maxWords < 1 {
		maxWords = 1
	}
	if overlapWords >= maxWords {
		overlapWords = maxWords / 4
	}

	words := strings.Fields(text)
	if len(words) <= maxWords {
		return []string{strings.Join(words, " ")}
	}

	var chunks []string
	step := maxWords - overlapWords
	if step < 1 {
		step = 1
	}

	for i := 0; i < len(words); i += step {
		end := i + maxWords
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(words) {
			break
		}
	}

	return chunks
}
