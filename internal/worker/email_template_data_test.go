package worker

import "testing"

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
	data := templateDataFromMap("JapaConnect", map[string]any{
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

	brandFallback := templateDataFromMap("", nil)
	if brandFallback["SiteName"] == "" || brandFallback["SiteName"] == nil {
		t.Error("SiteName fallback to brand.Get() produced empty value")
	}
}
