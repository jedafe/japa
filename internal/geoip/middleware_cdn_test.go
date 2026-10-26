package geoip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ADR-134: when MaxMind yields no country and a CDN provider is configured,
// the provider's country header fills the gap — but only for trusted-proxy
// peers, since a direct client can forge the header.

const (
	cdnCountryHeader = "CF-IPCountry"
	trustedPeerAddr  = "10.0.0.9:4433" // RFC1918 — inside the default trusted CIDRs
)

func cdnFallbackRequest(t *testing.T, remoteAddr, headerVal string) *http.Request {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), "GET", "/", nil)
	req.RemoteAddr = remoteAddr

	if headerVal != "" {
		req.Header.Set(cdnCountryHeader, headerVal)
	}

	return req
}

func TestCDNCountryFallback_TrustedPeer(t *testing.T) {
	t.Parallel()

	req := cdnFallbackRequest(t, trustedPeerAddr, "ng")

	gc := cdnCountryFallback(GeoContext{IsValid: false}, req, cdnCountryHeader)
	if !gc.IsValid || gc.CountryCode != "NG" {
		t.Fatalf("expected NG valid, got %+v", gc)
	}
}

func TestCDNCountryFallback_UntrustedPeerIgnored(t *testing.T) {
	t.Parallel()

	req := cdnFallbackRequest(t, "8.8.8.8:4433", "NG") // public IP — not trusted

	gc := cdnCountryFallback(GeoContext{IsValid: false}, req, cdnCountryHeader)
	if gc.IsValid || gc.CountryCode != "" {
		t.Fatalf("forged header from untrusted peer must be ignored, got %+v", gc)
	}
}

func TestCDNCountryFallback_SentinelsRejected(t *testing.T) {
	t.Parallel()

	for _, sentinel := range []string{"XX", "T1", "XX1", "1234", "N"} {
		req := cdnFallbackRequest(t, trustedPeerAddr, sentinel)

		gc := cdnCountryFallback(GeoContext{}, req, cdnCountryHeader)
		if gc.CountryCode != "" {
			t.Fatalf("sentinel %q must not produce a country, got %+v", sentinel, gc)
		}
	}
}

func TestCDNCountryFallback_ResolvedCountryWins(t *testing.T) {
	t.Parallel()

	// Local DB result stays authoritative — the header only fills gaps.
	req := cdnFallbackRequest(t, trustedPeerAddr, "GB")

	in := GeoContext{CountryCode: "NG", CountryName: "Ghana", IsValid: true}

	gc := cdnCountryFallback(in, req, cdnCountryHeader)
	if gc.CountryCode != "NG" || gc.CountryName != "Ghana" {
		t.Fatalf("resolved geo must not be overridden, got %+v", gc)
	}
}

func TestCDNCountryFallback_NoHeaderConfigured(t *testing.T) {
	t.Parallel()

	req := cdnFallbackRequest(t, trustedPeerAddr, "NG")

	gc := cdnCountryFallback(GeoContext{}, req, "")
	if gc.CountryCode != "" || gc.IsValid {
		t.Fatalf("no provider header configured — must be ignored, got %+v", gc)
	}
}

func TestNormalizeCountryCode(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"ng", "NG"},
		{" GB ", "GB"},
		{"XX", ""},
		{"T1", ""},
		{"NGA", ""},
		{"N1", ""},
		{"", ""},
	}

	for _, tc := range cases {
		if got := normalizeCountryCode(tc.in); got != tc.want {
			t.Errorf("normalizeCountryCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
