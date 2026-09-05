package changedetector

import "testing"

func TestCompare(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		newHash   string
		priorHash string
		want      Decision
	}{
		{"first crawl", "abc", "", DecisionFirstCrawl},
		{"unchanged", "abc", "abc", DecisionUnchanged},
		{"changed", "abc", "xyz", DecisionChanged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := Compare(tc.newHash, tc.priorHash); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
