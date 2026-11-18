package attestationguide

import "testing"

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
