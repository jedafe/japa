package newsletter

import "testing"

func TestIsTrackingScanner(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_IGNORE_UA", "")

	cases := []struct {
		ua   string
		want bool
	}{
		{"GoogleImageProxy; (+http://www.google.com/ggp)", true},
		{"Mozilla/5.0 (compatible; Proofpoint Link Scanner)", true},
		{"Barracuda Networks", true},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15", false},
		{"", false},
		{"okhttp/4.12.0", false},
	}

	for _, tc := range cases {
		if got := IsTrackingScanner(tc.ua); got != tc.want {
			t.Errorf("IsTrackingScanner(%q) = %v, want %v", tc.ua, got, tc.want)
		}
	}
}

func TestIsTrackingScannerEnvExtension(t *testing.T) {
	t.Setenv("JC_NEWSLETTER_TRACKING_IGNORE_UA", " CorpGW ,, ")

	if !IsTrackingScanner("corpgw/2.1 link-check") {
		t.Error("env-supplied substring should match case-insensitively")
	}

	if IsTrackingScanner("Mozilla/5.0") {
		t.Error("normal UA must not match")
	}
}
