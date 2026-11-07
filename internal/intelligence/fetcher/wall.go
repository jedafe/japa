package fetcher

import (
	"net/http"
	"strings"
)

// DetectWall inspects a response for anti-bot / challenge-page signatures —
// Cloudflare, DataDome, PerimeterX/HUMAN, Incapsula, Akamai, Sucuri, AWS
// WAF, and generic "verify you are human" interstitials. It returns the
// matched signature name (for logs and the channel's last_error_message)
// and true when the response is a wall rather than content.
//
// Walls arrive two ways: error statuses carrying a challenge body (403/503)
// and deceptive 200s whose body IS the challenge. Both must be caught —
// the second case is the dangerous one: a challenge document stored as
// content would poison the channel's change-detection baseline and waste
// extraction spend on an interstitial.
func DetectWall(status int, headers http.Header, body []byte, contentType string) (string, bool) {
	if sig := wallHeaderSignature(status, headers); sig != "" {
		return sig, true
	}

	return wallBodySignature(body, contentType)
}

// wallHeaderSignature matches vendor headers that appear only on
// challenged/denied responses. Cookies (_abck, incap_ses_, datadome=) are
// deliberately NOT signals — vendors set them on normal responses too, so
// a header match must be decisive on its own.
func wallHeaderSignature(status int, headers http.Header) string {
	if headers == nil {
		return ""
	}

	// Cloudflare's explicit challenge marker — present only on
	// interstitial responses, never on passthrough traffic.
	if v := headers.Get("CF-Mitigated"); strings.Contains(strings.ToLower(v), "challenge") {
		return "cloudflare-mitigated"
	}

	if status < http.StatusBadRequest {
		return ""
	}

	// Vendor headers that are only meaningful on an error status — some
	// appear on normal 200s as well, so they cannot fire alone on 2xx.
	switch {
	case headers.Get("X-DataDome") != "":
		return "datadome-header"
	case headers.Get("X-Iinfo") != "":
		return "incapsula-header"
	case strings.Contains(strings.ToLower(headers.Get("X-CDN")), "incapsula"):
		return "incapsula-header"
	}

	server := strings.ToLower(headers.Get("Server"))
	if status == http.StatusForbidden || status == http.StatusServiceUnavailable {
		switch {
		case strings.Contains(server, "imperva"):
			return "imperva-denied"
		case strings.Contains(server, "akamai"):
			return "akamai-denied"
		}
	}

	return ""
}

// wallScanBytes caps how deep into a body the signature scan looks —
// challenge markers live at the head of the document.
const wallScanBytes = 64 * 1024

// wallSignatures are (name, required-substrings) pairs checked against the
// lowercased body head: every listed substring must appear for the
// signature to fire. Multi-token requirements keep ambiguous words
// ("akamai", "datadome", "challenge") from false-flagging pages that
// merely mention them in prose. Single-token entries are vendor markers
// that never appear in ordinary page content.
var wallSignatures = []struct {
	name  string
	allOf []string
}{
	{"cloudflare-challenge", []string{"_cf_chl"}},
	{"cloudflare-challenge", []string{"challenge-platform"}},
	{"cloudflare-wait", []string{"just a moment", "cloudflare"}},
	{"cloudflare-wait", []string{"checking your browser"}},
	{"cloudflare-block", []string{"attention required", "cloudflare"}},
	{"datadome-captcha", []string{"captcha-delivery.com"}},
	{"datadome", []string{"datadome", "captcha"}},
	{"perimeterx", []string{"px-captcha"}},
	{"perimeterx", []string{"_pxoncontextloaded"}},
	{"perimeterx-block", []string{"pardon our interruption"}},
	{"human-challenge", []string{"human challenge", "captcha"}},
	{"incapsula-denied", []string{"incapsula incident"}},
	{"incapsula-denied", []string{"request unsuccessful", "incapsula"}},
	{"akamai-sensor", []string{"akamai", "sensor"}},
	{"akamai-denied", []string{"access denied", "reference #"}},
	{"distil", []string{"distil", "captcha"}},
	{"sucuri", []string{"sucuri website firewall"}},
	{"sucuri", []string{"access denied - sucuri"}},
	{"aws-waf", []string{"awswaf"}},
	{"verify-human", []string{"verify you are human"}},
	{"verify-human", []string{"verify that you are human"}},
	{"are-you-robot", []string{"are you a robot", "captcha"}},
	{"ddos-guard", []string{"ddos protection by"}},
	{"bot-check", []string{"checking if you are a bot"}},
}

// wallBodySignature scans HTML-ish bodies for challenge markers. Non-HTML
// payloads are skipped — a wall never arrives as valid JSON/XML, and
// substring matches inside binary documents (PDFs, images) would be noise.
func wallBodySignature(body []byte, contentType string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	if !isHTMLish(contentType) && !looksLikeHTML(body) {
		return "", false
	}

	head := body
	if len(head) > wallScanBytes {
		head = head[:wallScanBytes]
	}

	text := strings.ToLower(string(head))

	for _, sig := range wallSignatures {
		match := true

		for _, needle := range sig.allOf {
			if !strings.Contains(text, needle) {
				match = false
				break
			}
		}

		if match {
			return sig.name, true
		}
	}

	return "", false
}

// isHTMLish reports whether a content type plausibly carries HTML. Empty
// and text/plain types pass — mislabeled responses are sniffed by the
// body check anyway, and excluding them would hide walls served without
// a proper content type.
func isHTMLish(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))

	return ct == "" || strings.Contains(ct, "html") || strings.Contains(ct, "text/plain")
}

// CollapseConfig holds the content-collapse guard's bounds — the residual
// wall case a signature scan cannot see (unknown challenge vendors,
// silent WAF stubs). The worker substitutes system_settings overrides
// (intel.collapse_min_prev_bytes / collapse_ratio_pct / collapse_max_bytes).
type CollapseConfig struct {
	// MinPrevBytes is the smallest previous-capture size worth comparing —
	// a page that was never substantial has no baseline to collapse from.
	MinPrevBytes int64
	// RatioPct is the shrink threshold: a new capture below this percent
	// of the previous body is suspect.
	RatioPct int64
	// MaxBytes caps the absolute size of a collapsed capture — challenge
	// stubs are small; a page that merely slimmed to 30KB is content.
	MaxBytes int64
}

// DefaultCollapseConfig is the compiled-in guard: prior captures of 20KB+
// shrinking to under 15% AND under 8KB absolute read as a wall/stub, not
// editorial change.
var DefaultCollapseConfig = CollapseConfig{
	MinPrevBytes: 20 * 1024,
	RatioPct:     15,
	MaxBytes:     8 * 1024,
}

// LooksCollapsed reports whether an HTML capture is suspiciously thin
// relative to the channel's previous document. Only HTML bodies can
// collapse — manifests, feeds, JSON and binary documents are legitimately
// small or differently shaped.
func (c CollapseConfig) LooksCollapsed(prevLen int64, contentType string, body []byte) bool {
	if c.MinPrevBytes <= 0 || c.RatioPct <= 0 {
		return false
	}

	if prevLen < c.MinPrevBytes {
		return false
	}

	newLen := int64(len(body))
	if newLen == 0 || newLen >= c.MaxBytes {
		return false
	}

	if !isHTMLish(contentType) && !looksLikeHTML(body) {
		return false
	}

	return newLen*100 < prevLen*c.RatioPct
}
