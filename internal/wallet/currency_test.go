// currency_test.go — §58 conversion-math unit tests: numeric rates
// (001185) + ISO minor-unit exponents (JPY 0-dp, KWD 3-dp).
package wallet

import (
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

func cur(code string, cpu float64) gen.Currency {
	symbols := map[string]string{"USD": "$", "NGN": "₦", "JPY": "¥", "KWD": "KWD"}
	return gen.Currency{Code: code, CreditsPerUnit: cpu, Symbol: symbols[code]}
}

func TestFiatToCredits(t *testing.T) {
	cases := []struct {
		name  string
		minor int64
		cur   gen.Currency
		want  int
	}{
		{"usd 1 dollar", 100, cur("USD", 100), 100},
		{"usd cents exact", 1, cur("USD", 100), 1},
		{"ngn below one credit", 700, cur("NGN", 0.065), 0},
		{"ngn one credit", 1539, cur("NGN", 0.065), 1},
		{"jpy zero-decimal", 100, cur("JPY", 0.65), 65},
		{"kwd three-decimal", 1000, cur("KWD", 325), 325},
		{"inactive rate", 100, cur("USD", 0), 0},
	}
	for _, tc := range cases {
		if got := FiatToCredits(tc.minor, tc.cur); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCreditsToFiat(t *testing.T) {
	cases := []struct {
		name    string
		credits int
		cur     gen.Currency
		want    int64
	}{
		{"usd 100 credits", 100, cur("USD", 100), 100},
		{"ngn 1 credit in kobo", 1, cur("NGN", 0.065), 1538},
		{"jpy whole yen", 65, cur("JPY", 0.65), 100},
		{"kwd fils", 325, cur("KWD", 325), 1000},
		{"zero rate", 100, cur("USD", 0), 0},
	}
	for _, tc := range cases {
		if got := CreditsToFiat(tc.credits, tc.cur); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestFormatFiat(t *testing.T) {
	if got := FormatFiat(1538, cur("NGN", 0.065)); got != "₦15.38" {
		t.Errorf("NGN: got %q", got)
	}
	if got := FormatFiat(15, cur("JPY", 0.65)); got != "¥15" {
		t.Errorf("JPY: got %q", got)
	}
	if got := FormatFiat(2500, cur("KWD", 325)); got != "KWD2.500" {
		t.Errorf("KWD: got %q", got)
	}
}
