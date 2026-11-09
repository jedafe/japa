package customfields

import "testing"

func TestValidFieldConfig(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"object", `{"visible_if":{"all":[]}}`, true},
		{"empty object", `{}`, true},
		{"array", `[1,2]`, false},
		{"scalar", `5`, false},
		{"string", `"x"`, false},
		{"bad json", `{oops`, false},
	}
	for _, c := range cases {
		if got := validFieldConfig([]byte(c.in)); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
