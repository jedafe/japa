// Package countrydata provides lookup tables for country names, codes, and
// adjectives used by template literal expressions.
package countrydata

import "testing"

func TestAdjectiveFor(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"NG", "Nigerian"},
		{"Nigeria", "Nigerian"},
		{"GB", "British"},
		{"France", "French"},
		{"france", "French"},
		{"NO SUCH", ""},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()

			got := AdjectiveFor(c.in)
			if got != c.want {
				t.Errorf("AdjectiveFor(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCodeForName(t *testing.T) {
	t.Parallel()

	if got := CodeForName("Nigeria"); got != "NG" {
		t.Errorf("CodeForName(\"Nigeria\") = %q; want NG", got)
	}
}

func TestNameForCode(t *testing.T) {
	t.Parallel()

	if got := NameForCode("NG"); got != "Nigeria" {
		t.Errorf("NameForCode(\"NG\") = %q; want Nigeria", got)
	}
}
