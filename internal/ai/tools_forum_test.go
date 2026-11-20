package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBodyToTipTap_UnwrapsStringEncodedDoc(t *testing.T) {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`
	raw := json.RawMessage(`"` + strings.ReplaceAll(doc, `"`, `\"`) + `"`)

	got, err := bodyToTipTap(raw)
	if err != nil {
		t.Fatalf("bodyToTipTap error: %v", err)
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
		t.Fatalf("content len = %d, want 1", len(content))
	}
}

func TestBodyToTipTap_UnwrapsStringEncodedArray(t *testing.T) {
	arr := `[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]`
	raw := json.RawMessage(`"` + strings.ReplaceAll(arr, `"`, `\"`) + `"`)

	got, err := bodyToTipTap(raw)
	if err != nil {
		t.Fatalf("bodyToTipTap error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out["type"] != "doc" {
		t.Fatalf("type = %v, want doc", out["type"])
	}
	if len(out["content"].([]any)) != 1 {
		t.Fatal("array should be wrapped as doc content")
	}
}

func TestBodyToTipTap_PlainTextStillWraps(t *testing.T) {
	got, err := bodyToTipTap(json.RawMessage(`"just a comment"`))
	if err != nil {
		t.Fatalf("bodyToTipTap error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out["type"] != "doc" {
		t.Fatalf("type = %v, want doc", out["type"])
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), "just a comment") {
		t.Fatal("plain text should be wrapped as paragraph text")
	}
}

func TestBodyToTipTap_ObjectPassesThrough(t *testing.T) {
	doc := `{"type":"doc","content":[]}`
	got, err := bodyToTipTap(json.RawMessage(doc))
	if err != nil {
		t.Fatalf("bodyToTipTap error: %v", err)
	}
	if string(got) != doc {
		t.Fatalf("object should pass through unchanged, got %s", got)
	}
}
