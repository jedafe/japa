package worker

import (
	"html/template"
	"testing"
)

// TestSnakeToPascal_Initialisms guards the seeded-template variable
// contract: declared names use full initialisms (SiteURL, TransactionID,
// TicketID) and html/template map lookup is exact-match — a naive
// title-case produced SiteUrl/TransactionId and rendered blank.
func TestSnakeToPascal_Initialisms(t *testing.T) {
	cases := map[string]string{
		"user_name":       "UserName",
		"site_url":        "SiteURL",
		"action_url":      "ActionURL",
		"support_url":     "SupportURL",
		"certificate_url": "CertificateURL",
		"content_url":     "ContentURL",
		"transaction_id":  "TransactionID",
		"ticket_id":       "TicketID",
		"ip_address":      "IPAddress",
		"digest_content":  "DigestContent",
		"plan_name":       "PlanName",
		"new_username":    "NewUsername",
		"expiry_date":     "ExpiryDate",
		"failure_reason":  "FailureReason",
	}

	for in, want := range cases {
		if got := snakeToPascal(in); got != want {
			t.Errorf("snakeToPascal(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPascalizeTemplateValue_PointerDeref guards BUG-040: nullable *string
// fields must unwrap — a passed-through pointer renders as 0xc000… in the
// template.
func TestPascalizeTemplateValue_PointerDeref(t *testing.T) {
	device := "iPhone 17"
	if got := pascalizeTemplateValue(&device); got != "iPhone 17" {
		t.Errorf("non-nil *string: got %v, want iPhone 17", got)
	}

	var nilStr *string
	if got := pascalizeTemplateValue(nilStr); got != "" {
		t.Errorf("nil *string: got %v, want empty", got)
	}
}

// TestTemplateDataFromMap checks the render context: SiteName is always
// injected (brand fallback when the producer omits template_site_name)
// and nested maps are pascalized recursively.
func TestTemplateDataFromMap(t *testing.T) {
	device := "Safari"
	data := templateDataFromMap("JapaConnect", "notification", map[string]any{
		"user_name":   "jane",
		"device_info": &device,
		"site_url":    "https://japa.example",
	})

	if data["SiteName"] != "JapaConnect" {
		t.Errorf("SiteName = %v, want JapaConnect", data["SiteName"])
	}
	if data["UserName"] != "jane" {
		t.Errorf("UserName = %v, want jane", data["UserName"])
	}
	if data["DeviceInfo"] != "Safari" {
		t.Errorf("DeviceInfo = %v, want Safari", data["DeviceInfo"])
	}
	if data["SiteURL"] != "https://japa.example" {
		t.Errorf("SiteURL = %v, want https://japa.example", data["SiteURL"])
	}

	brandFallback := templateDataFromMap("", "notification", nil)
	if brandFallback["SiteName"] == "" || brandFallback["SiteName"] == nil {
		t.Error("SiteName fallback to brand.Get() produced empty value")
	}
}

// Trusted-HTML keys are slug-scoped: 'content' is only unescaped inside the
// newsletter campaign wrapper, 'digest_content' only inside the digest
// slugs — under any other slug the same key renders as an escaped string so
// a future producer passing user input cannot inject markup.
func TestTemplateDataTrustedHTMLSlugScope(t *testing.T) {
	mk := func(slug string) map[string]any {
		return templateDataFromMap("", slug, map[string]any{
			"content":        "<p>x</p>",
			"digest_content": "<p>x</p>",
		})
	}

	if _, ok := mk("newsletter")["Content"].(template.HTML); !ok {
		t.Error("content should be trusted HTML inside newsletter")
	}
	if _, ok := mk("newsletter")["DigestContent"].(template.HTML); ok {
		t.Error("digest_content must NOT be trusted inside newsletter")
	}
	if _, ok := mk("weekly_digest")["DigestContent"].(template.HTML); !ok {
		t.Error("digest_content should be trusted HTML inside weekly_digest")
	}
	if _, ok := mk("weekly_digest")["Content"].(template.HTML); ok {
		t.Error("content must NOT be trusted inside weekly_digest")
	}
	for _, slug := range []string{"notification", "welcome", ""} {
		if _, ok := mk(slug)["Content"].(template.HTML); ok {
			t.Errorf("content must stay escaped under slug %q", slug)
		}
		if _, ok := mk(slug)["DigestContent"].(template.HTML); ok {
			t.Errorf("digest_content must stay escaped under slug %q", slug)
		}
	}
}
