package wiki

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeWikiContent_UnwrapsStringEncodedDoc(t *testing.T) {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`
	raw := json.RawMessage(`"` + strings.ReplaceAll(doc, `"`, `\"`) + `"`)

	got, err := normalizeWikiContent(raw)
	if err != nil {
		t.Fatalf("normalizeWikiContent error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out["type"] != "doc" {
		t.Fatalf("type = %v, want doc — string-encoded doc should unwrap, not become literal text", out["type"])
	}
	content, _ := out["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1 (the real paragraph, not a wrapper holding literal JSON)", len(content))
	}
}

func TestNormalizeWikiContent_PlainStringStillWraps(t *testing.T) {
	got, err := normalizeWikiContent(json.RawMessage(`"plain markdown text"`))
	if err != nil {
		t.Fatalf("normalizeWikiContent error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), "plain markdown text") {
		t.Fatal("plain string should be wrapped as paragraph text")
	}
}

func TestNormalizeWikiContent_ObjectPathUnchanged(t *testing.T) {
	doc := `{"type":"doc","content":[]}`
	got, err := normalizeWikiContent(json.RawMessage(doc))
	if err != nil {
		t.Fatalf("normalizeWikiContent error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out["type"] != "doc" {
		t.Fatalf("type = %v, want doc", out["type"])
	}
}

func TestNormalizeWikiContent_StringEncodedMissingType(t *testing.T) {
	// A JSON object without "type" is not a doc — must error, matching the
	// object path rather than becoming literal text.
	raw := json.RawMessage(`"{\"foo\":\"bar\"}"`)
	if _, err := normalizeWikiContent(raw); err == nil {
		t.Fatal("string-encoded object without type should error like the object path")
	}
}
