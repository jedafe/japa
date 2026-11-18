package attestationguide

import (
	"testing"

	"github.com/jedafe/japa/internal/ai"
)

func TestIsJSONArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty array", `[]`, true},
		{"array of objects", `[{"step":1},{"step":2}]`, true},
		{"whitespace padded", "  \n\t[1, 2]  \n", true},
		{"json object", `{"a":1}`, false},
		{"json string", `"text"`, false},
		{"json number", `42`, false},
		{"json null literal", `null`, false},
		{"json true", `true`, false},
		{"empty input", ``, false},
		{"whitespace only", "   ", false},
		{"invalid json starting with bracket", `[{`, false},
		{"invalid json not bracket", `not-json`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJSONArray([]byte(tt.input)); got != tt.want {
				t.Errorf("isJSONArray(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePathwayStepsFromAIResponse(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			"plain text key is wrapped as one step",
			map[string]any{"text": "Notarize the document."},
			`[{"step":"Notarize the document."}]`,
		},
		{
			"content key is wrapped as one step",
			map[string]any{"content": "Do the thing."},
			`[{"step":"Do the thing."}]`,
		},
		{
			"json array in text passes through verbatim",
			map[string]any{"text": `[{"step":"a"},{"step":"b"}]`},
			`[{"step":"a"},{"step":"b"}]`,
		},
		{
			"fenced json array passes through",
			map[string]any{"text": "```json\n[{\"step\":\"a\"}]\n```"},
			`[{"step":"a"}]`,
		},
		{
			"openai choices shape is extracted",
			map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "Step one."}}}},
			`[{"step":"Step one."}]`,
		},
		{
			"empty response yields empty array",
			map[string]any{},
			`[]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePathwayStepsFromAIResponse(ai.Response{Raw: tt.raw})
			if !isJSONArray(got) {
				t.Fatalf("parsePathwayStepsFromAIResponse(%v) = %s — not a JSON array", tt.raw, got)
			}
			if string(got) != tt.want {
				t.Errorf("parsePathwayStepsFromAIResponse(%v) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}
