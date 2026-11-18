package attestationguide

import (
	"encoding/json"
	"testing"
)

func TestNormalizeEditorialSteps(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"official_url renamed to url",
			`[{"step":"Embassy legalization","official_url":"https://mofa.gov.ae","turnaround_days":7}]`,
			`[{"step":"Embassy legalization","turnaround_days":7,"url":"https://mofa.gov.ae"}]`,
		},
		{
			"existing url wins over official_url",
			`[{"url":"https://a.gov","official_url":"https://b.gov"}]`,
			`[{"url":"https://a.gov"}]`,
		},
		{
			"no official_url returns input verbatim",
			`[{"step":"s","url":"https://x.gov"}]`,
			`[{"step":"s","url":"https://x.gov"}]`,
		},
		{
			"min/max objects and items arrays pass through",
			`[{"step":"s","fee_estimate":{"min":5000,"max":20000,"currency":"NGN"},"turnaround_days":{"min":5,"max":14},"what_to_bring":["a","b"],"official_url":"https://x.gov"}]`,
			`[{"step":"s","fee_estimate":{"min":5000,"max":20000,"currency":"NGN"},"turnaround_days":{"min":5,"max":14},"what_to_bring":["a","b"],"url":"https://x.gov"}]`,
		},
		{
			"non-object items survive",
			`["just a note",{"official_url":"https://x.gov"}]`,
			`["just a note",{"url":"https://x.gov"}]`,
		},
		{
			"empty array",
			`[]`,
			`[]`,
		},
		{
			"not an array returns input",
			`{"a":1}`,
			`{"a":1}`,
		},
		{
			"empty input",
			``,
			``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEditorialSteps(json.RawMessage(tt.input))
			if tt.input == "" {
				if len(got) != 0 {
					t.Errorf("normalizeEditorialSteps(empty) = %q, want empty", got)
				}
				return
			}

			// Compare semantically — map marshal order is not stable.
			var gotV, wantV any
			if err := json.Unmarshal(got, &gotV); err != nil {
				t.Fatalf("normalizeEditorialSteps(%q) produced invalid JSON: %v", tt.input, err)
			}
			if err := json.Unmarshal(json.RawMessage(tt.want), &wantV); err != nil {
				t.Fatalf("test want fixture invalid: %v", err)
			}
			gotB, _ := json.Marshal(gotV)
			wantB, _ := json.Marshal(wantV)
			if string(gotB) != string(wantB) {
				t.Errorf("normalizeEditorialSteps(%q) = %s, want %s", tt.input, gotB, wantB)
			}
		})
	}
}
