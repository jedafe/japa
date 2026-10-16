package sources

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestEntityTypesFromProviderConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", ``, nil},
		{"empty object", `{}`, nil},
		{"no key", `{"endpoint":"https://x"}`, nil},
		{"empty array", `{"entity_types":[]}`, []string{}},
		{"values", `{"entity_types":["scholarship","job"]}`, []string{"scholarship", "job"}},
		{"malformed json", `[broken`, nil},
		{"wrong type", `{"entity_types":"job"}`, nil},
		{"null", `{"entity_types":null}`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := EntityTypesFromProviderConfig([]byte(tt.raw))
			if len(got) != len(tt.want) {
				t.Fatalf("EntityTypesFromProviderConfig(%q) = %v, want %v", tt.raw, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("EntityTypesFromProviderConfig(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSetEntityTypesInProviderConfig(t *testing.T) {
	t.Parallel()

	t.Run("merges into existing document", func(t *testing.T) {
		t.Parallel()

		out, err := SetEntityTypesInProviderConfig(
			[]byte(`{"endpoint":"https://api.example.com/x","method":"GET"}`),
			[]string{"scholarship", "job"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var doc map[string]json.RawMessage
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("output not JSON: %v", err)
		}

		if _, ok := doc["endpoint"]; !ok {
			t.Error("sibling key endpoint was dropped")
		}

		if _, ok := doc["method"]; !ok {
			t.Error("sibling key method was dropped")
		}

		var types []string
		if err := json.Unmarshal(doc["entity_types"], &types); err != nil {
			t.Fatalf("entity_types not an array: %v", err)
		}

		if !slices.Equal(types, []string{"scholarship", "job"}) {
			t.Errorf("entity_types = %v", types)
		}
	})

	t.Run("empty slice removes the key", func(t *testing.T) {
		t.Parallel()

		out, err := SetEntityTypesInProviderConfig(
			[]byte(`{"endpoint":"https://x","entity_types":["job"]}`),
			nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var doc map[string]json.RawMessage
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("output not JSON: %v", err)
		}

		if _, ok := doc["entity_types"]; ok {
			t.Error("entity_types key should have been removed")
		}

		if _, ok := doc["endpoint"]; !ok {
			t.Error("sibling key endpoint was dropped")
		}
	})

	t.Run("nil input produces fresh document", func(t *testing.T) {
		t.Parallel()

		out, err := SetEntityTypesInProviderConfig(nil, []string{"news"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var doc map[string]json.RawMessage
		if err := json.Unmarshal(out, &doc); err != nil {
			t.Fatalf("output not JSON: %v", err)
		}

		if _, ok := doc["entity_types"]; !ok {
			t.Error("entity_types missing")
		}
	})

	t.Run("corrupt document returns error", func(t *testing.T) {
		t.Parallel()

		if _, err := SetEntityTypesInProviderConfig([]byte(`[1,2]`), []string{"news"}); err == nil {
			t.Error("expected error for non-object provider_config")
		}
	})
}
