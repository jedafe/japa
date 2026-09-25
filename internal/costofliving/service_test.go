package costofliving

import "testing"

func TestAnyToFloat(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"float64", 1200.5, 1200.5, true},
		{"int", 42, 42, true},
		{"numeric string", "1600", 1600, true},
		{"currency string", "$1, check", 0, false},
		{"usd prefix", "USD 950", 950, true},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, c := range cases {
		got, ok := anyToFloat(c.in)
		if ok != c.ok {
			t.Errorf("%s: expected ok=%v, got %v", c.name, c.ok, ok)
			continue
		}

		if ok && got != c.want {
			t.Errorf("%s: expected %v, got %v", c.name, c.want, got)
		}
	}
}
