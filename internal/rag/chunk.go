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

// ChunkSpecDoc splits markdown spec text on heading boundaries (H1–H4) and
// packs consecutive sections into chunks of at most maxTokens. A section
// larger than maxTokens falls back to ChunkText's overlapping word windows
// so no section is silently truncated. Used by the §121 spec corpus indexer —
// heading-aligned chunks keep each retrieval result self-contained (a section
// heading + its body) rather than cutting mid-argument.
//
// Ref: japaconnect-engineering/sections/121-engineering-spec-corpus.md
func ChunkSpecDoc(text string, maxTokens int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var sections []string
	var current strings.Builder

	flush := func() {
		if s := strings.TrimSpace(current.String()); s != "" {
			sections = append(sections, s)
		}

		current.Reset()
	}

	for line := range strings.Lines(text) {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			marker := strings.TrimRight(trimmed, " \t\r\n")
			if h, _, ok := strings.Cut(marker, " "); ok && len(h) <= 4 && strings.Trim(h, "#") == "" {
				// Heading line (all '#' chars, ≤4) → start a new section.
				flush()
			}
		}

		current.WriteString(line)
	}

	flush()

	// Approximate tokens: words / 1.3 — same convention as ChunkText.
	approxTokens := func(s string) int {
		return int(float64(len(strings.Fields(s))) * 1.3)
	}

	var chunks []string
	var packed strings.Builder
	packedTokens := 0

	for _, sec := range sections {
		secTokens := approxTokens(sec)

		if secTokens > maxTokens {
			// Oversized section: emit whatever is packed, then word-window it.
			if s := strings.TrimSpace(packed.String()); s != "" {
				chunks = append(chunks, s)
				packed.Reset()
				packedTokens = 0
			}

			chunks = append(chunks, ChunkText(sec, maxTokens, maxTokens/5)...)
			continue
		}

		if packedTokens+secTokens > maxTokens {
			chunks = append(chunks, strings.TrimSpace(packed.String()))
			packed.Reset()
			packedTokens = 0
		}

		if packed.Len() > 0 {
			packed.WriteString("\n\n")
		}

		packed.WriteString(sec)
		packedTokens += secTokens
	}

	if s := strings.TrimSpace(packed.String()); s != "" {
		chunks = append(chunks, s)
	}

	return chunks
}
