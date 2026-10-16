package schemas

import "testing"

func TestHasExtractedValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"empty object", `{}`, false},
		{"invalid json", `not-json`, true},
		{"nested all null", `{"title":{"value":null,"confidence":1,"evidence":null},"country":{"value":null,"confidence":0}}`, false},
		{"nested with one value", `{"title":{"value":"DAAD EPOS","confidence":0.9,"evidence":"e"},"country":{"value":null}}`, true},
		{"flat seed shape", `{"title":"DAAD Scholarship Program 2","country":"Germany"}`, true},
		{"flat null leaf", `{"title":null,"country":null}`, false},
		{"nested object without value key", `{"meta":{"pages":3}}`, true},
		{"records wrapper", `{"records":[{"title":{"value":"X"}}]}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := HasExtractedValues([]byte(tc.raw)); got != tc.want {
				t.Errorf("HasExtractedValues(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
