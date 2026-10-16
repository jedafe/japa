package rag

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSpecEntityIDDeterministic(t *testing.T) {
	t.Parallel()

	a := SpecEntityID("sections/44-documentation-system.md")
	b := SpecEntityID("sections/44-documentation-system.md")
	c := SpecEntityID("sections/45-other.md")

	if !a.Valid || !b.Valid || !c.Valid {
		t.Fatal("spec entity IDs must be valid UUIDs")
	}

	if a != b {
		t.Error("same path must produce the same deterministic ID")
	}

	if a == c {
		t.Error("different paths must produce different IDs")
	}
}

func TestChunkSpecDocHeadingSplit(t *testing.T) {
	t.Parallel()

	doc := "# Title\n\nintro words here.\n\n## Alpha\n\nalpha body.\n\n## Beta\n\nbeta body.\n\n## Gamma\n\ngamma body."

	chunks := ChunkSpecDoc(doc, 1000)
	if len(chunks) != 1 {
		t.Fatalf("small doc should pack into one chunk, got %d", len(chunks))
	}

	if !strings.Contains(chunks[0], "## Beta") {
		t.Error("packed chunk must retain section headings")
	}
}

func TestChunkSpecDocOversizedSection(t *testing.T) {
	t.Parallel()

	big := "## Big\n\n" + strings.Repeat("word ", 800) // ~1040 tokens > maxTokens
	doc := "# T\n\nshort intro.\n\n" + big + "\n\n## Small\n\ntiny."

	chunks := ChunkSpecDoc(doc, 100)
	if len(chunks) < 3 {
		t.Fatalf("expected intro + split big section + small section, got %d chunks", len(chunks))
	}

	if !strings.HasPrefix(chunks[0], "# T") {
		t.Error("first chunk should be the small intro section")
	}

	if !strings.Contains(chunks[len(chunks)-1], "## Small") {
		t.Error("last chunk should be the trailing small section")
	}
}

func TestChunkSpecDocEmpty(t *testing.T) {
	t.Parallel()

	if got := ChunkSpecDoc("   \n\n", 100); got != nil {
		t.Fatalf("empty input must produce nil, got %v", got)
	}
}

// testFlagChecker satisfies the Service.checker FeatureChecker interface for
// flag-gating tests.
type testFlagChecker struct{ on bool }

func (c testFlagChecker) IsEnabled(_ context.Context, _ string) bool { return c.on }

func TestReindexSpecCorpusFlagOff(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	// checker=false → ErrSpecCorpusDisabled before any store/embedder use.
	svc.SetFeatureChecker(testFlagChecker{on: false})

	_, err := svc.ReindexSpecCorpus(t.Context(), t.TempDir())
	if !errors.Is(err, ErrSpecCorpusDisabled) {
		t.Fatalf("expected ErrSpecCorpusDisabled, got %v", err)
	}
}

func TestRetrievePrivilegedFlagOff(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	svc.SetFeatureChecker(testFlagChecker{on: false})

	_, err := svc.RetrievePrivileged(t.Context(), "q", "en", "", 5)
	if !errors.Is(err, ErrSpecCorpusDisabled) {
		t.Fatalf("expected ErrSpecCorpusDisabled, got %v", err)
	}
}
