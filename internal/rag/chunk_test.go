// Package rag — chunk_test.go tests the text chunking utility. These are
// pure-function tests with no database dependency.
package rag

import (
	"strings"
	"testing"
)

func TestChunkText_Empty(t *testing.T) {
	chunks := ChunkText("", 400, 80)
	if len(chunks) != 0 {
		t.Errorf("ChunkText(\"\") = %d chunks, want 0", len(chunks))
	}
}

func TestChunkText_ShortText(t *testing.T) {
	text := "This is a short text that fits in one chunk."

	chunks := ChunkText(text, 400, 80)
	if len(chunks) != 1 {
		t.Fatalf("ChunkText(short) = %d chunks, want 1", len(chunks))
	}

	if chunks[0] != text {
		t.Errorf("ChunkText(short)[0] = %q, want %q", chunks[0], text)
	}
}

func TestChunkText_LongText(t *testing.T) {
	// Generate a text with ~1000 words.
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word"
	}

	text := strings.Join(words, " ")

	// maxTokens=400 → maxWords ≈ 307. With 1000 words, we expect ~4-5 chunks.
	chunks := ChunkText(text, 400, 80)
	if len(chunks) < 3 {
		t.Errorf("ChunkText(long) = %d chunks, want at least 3", len(chunks))
	}

	// Each chunk should be at most ~307 words.
	for i, c := range chunks {
		wordCount := len(strings.Fields(c))
		if wordCount > 310 {
			t.Errorf("ChunkText(long)[%d] = %d words, want <= 310", i, wordCount)
		}
	}

	// Chunks should overlap (the first words of chunk[1] should appear in
	// the last words of chunk[0]).
	if len(chunks) >= 2 {
		firstChunkEnd := strings.Fields(chunks[0])
		secondChunkStart := strings.Fields(chunks[1])

		if len(firstChunkEnd) > 0 && len(secondChunkStart) > 0 {
			// Check that the overlap exists: the first word of chunk[1]
			// should appear somewhere in the last few words of chunk[0].
			overlapWord := secondChunkStart[0]
			found := false

			for i := max(0, len(firstChunkEnd)-80); i < len(firstChunkEnd); i++ {
				if firstChunkEnd[i] == overlapWord {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("ChunkText: no overlap between chunk 0 and 1")
			}
		}
	}
}

func TestChunkText_ZeroMaxTokens(t *testing.T) {
	text := "Some text here."
	chunks := ChunkText(text, 0, 0)

	if len(chunks) != 1 || chunks[0] != text {
		t.Errorf("ChunkText(maxTokens=0) = %v, want [%q]", chunks, text)
	}
}

func TestChunkText_AllChunksNonEmpty(t *testing.T) {
	words := make([]string, 500)
	for i := range words {
		words[i] = "test"
	}

	text := strings.Join(words, " ")

	chunks := ChunkText(text, 100, 20)
	for i, c := range chunks {
		if strings.TrimSpace(c) == "" {
			t.Errorf("ChunkText: chunk %d is empty", i)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}
