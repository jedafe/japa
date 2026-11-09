package customfields

import "testing"

func TestValidateFieldConfig(t *testing.T) {
	t.Parallel()

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
		{"valid visible_if", `{"visible_if":{"all":[{"field":"a","op":"eq","value":1}]}}`, true},
		{"valid jump", `{"jump":{"when":{"all":[{"op":"eq","value":"x"}]},"step":"s2"}}`, true},
		{"visible_if not object", `{"visible_if":"nope"}`, false},
		{"clause not object", `{"visible_if":{"all":[5]}}`, false},
		{"clause missing op", `{"visible_if":{"all":[{"field":"a"}]}}`, false},
		{"unknown op", `{"visible_if":{"all":[{"op":"between"}]}}`, false},
		{"in needs array value", `{"visible_if":{"all":[{"op":"in","value":"a"}]}}`, false},
		{"jump missing step", `{"jump":{"when":{"all":[{"op":"eq"}]}}}`, false},
		{"jump bad when", `{"jump":{"when":"x","step":"s2"}}`, false},
		{"scoring keys untouched", `{"weight":2,"option_scores":{"a":1}}`, true},
	}
	for _, c := range cases {
		err := validateFieldConfig([]byte(c.in))
		if got := err == nil; got != c.want {
			t.Errorf("%s: got valid=%v want %v (err=%v)", c.name, got, c.want, err)
		}
	}
}
