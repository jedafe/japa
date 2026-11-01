// affiliate_match_test.go — §125 click-time tracking injection helper.
// ResolveAffiliateLinkByURL itself needs a live store; its pure append logic
// is covered here, matching the package's helper-test convention.
package coupons

import "testing"

func TestAppendTrackingParam(t *testing.T) {
	cases := []struct {
		name, url, param, value, want string
	}{
		{"fresh query", "https://amazon.com/dp/x", "tag", "jc-20", "https://amazon.com/dp/x?tag=jc-20"},
		{"existing query", "https://amazon.com/dp/x?a=1", "tag", "jc-20", "https://amazon.com/dp/x?a=1&tag=jc-20"},
		{"foreign tag replaced", "https://amazon.com/dp/x?tag=other-1", "tag", "jc-20", "https://amazon.com/dp/x?tag=jc-20"},
		{"empty param", "https://amazon.com/dp/x", "", "jc-20", "https://amazon.com/dp/x"},
		{"empty value", "https://amazon.com/dp/x", "tag", "", "https://amazon.com/dp/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := appendTrackingParam(tc.url, tc.param, tc.value); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
