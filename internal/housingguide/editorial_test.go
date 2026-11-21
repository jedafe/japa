package housingguide

import (
	"encoding/json"
	"testing"
)

func TestNormalizeRentRanges(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"tiers map to §96 object shape",
			`[{"tier":"city_centre","studio":"1200","one_bed":"1800","two_bed":"2600","currency":"usd"}]`,
			`{"city_centre":{"studio":"1200","1bed":"1800","2bed":"2600","currency":"USD"}}`,
		},
		{
			"multiple tiers",
			`[{"tier":"inner","studio":"800","one_bed":"1200"},{"tier":"outer","studio":"500","two_bed":"900","currency":"CAD"}]`,
			`{"inner":{"studio":"800","1bed":"1200"},"outer":{"studio":"500","2bed":"900","currency":"CAD"}}`,
		},
		{
			"rows without tier are skipped",
			`[{"studio":"100"},{"tier":"inner","studio":"900"}]`,
			`{"inner":{"studio":"900"}}`,
		},
		{
			"tier-only row is skipped",
			`[{"tier":"inner"}]`,
			``,
		},
		{
			"non-array input yields nil",
			`{"inner":{"studio":"900"}}`,
			``,
		},
		{
			"empty input yields nil",
			``,
			``,
		},
		{
			"bad currency length dropped",
			`[{"tier":"inner","studio":"700","currency":"dollars"}]`,
			`{"inner":{"studio":"700"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRentRanges(json.RawMessage(tt.input))
			if tt.want == "" {
				if got != nil {
					t.Errorf("normalizeRentRanges(%q) = %s, want nil", tt.input, got)
				}
				return
			}
			var gotV, wantV any
			if err := json.Unmarshal(got, &gotV); err != nil {
				t.Fatalf("normalizeRentRanges(%q) produced invalid JSON: %v", tt.input, err)
			}
			if err := json.Unmarshal(json.RawMessage(tt.want), &wantV); err != nil {
				t.Fatalf("want fixture invalid: %v", err)
			}
			gotB, _ := json.Marshal(gotV)
			wantB, _ := json.Marshal(wantV)
			if string(gotB) != string(wantB) {
				t.Errorf("normalizeRentRanges(%q) = %s, want %s", tt.input, gotB, wantB)
			}
		})
	}
}

func TestGuideStringList(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{"textarea one per line", "phantom listings\n advance-fee fraud\n", []string{"phantom listings", "advance-fee fraud"}},
		{"textarea windows newlines", "a\r\nb", []string{"a", "b"}},
		{"json string array in textarea", `["a","b"]`, []string{"a", "b"}},
		{"native array", []any{"a", " b ", "", 42}, []string{"a", "b"}},
		{"empty string", "", nil},
		{"non-string scalar", 42, nil},
		{"nil", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guideStringList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("guideStringList(%v) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("guideStringList(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsJSONArray(t *testing.T) {
	if !isJSONArray([]byte(`[{"name":"x"}]`)) {
		t.Error("isJSONArray(array) = false")
	}
	if isJSONArray([]byte(`{"a":1}`)) {
		t.Error("isJSONArray(object) = true")
	}
	if isJSONArray(nil) {
		t.Error("isJSONArray(nil) = true")
	}
}
