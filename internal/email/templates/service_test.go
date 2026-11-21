package templates

import (
	"context"
	"testing"
)

func TestPascalToSnake(t *testing.T) {
	cases := map[string]string{
		"ActionURL":         "action_url",
		"LogoURL":           "logo_url",
		"SiteURL":           "site_url",
		"SiteName":          "site_name",
		"UserName":          "user_name",
		"TicketID":          "ticket_id",
		"TransactionID":     "transaction_id",
		"DigestContent":     "digest_content",
		"AppellantUsername": "appellant_username",
		"Expiry":            "expiry",
	}

	for in, want := range cases {
		if got := pascalToSnake(in); got != want {
			t.Errorf("pascalToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

// SampleTemplateData must produce a value for every declared variable
// (snake_case key) so test sends never render <no value> — while leaving
// site_name/logo_url to the worker's own resolution.
func TestSampleTemplateData(t *testing.T) {
	s := &Service{} // nil db → env/example.com site-base fallback

	vars := []byte(`[
		{"name": "SiteName"},
		{"name": "LogoURL"},
		{"name": "UserName"},
		{"name": "ActionURL"},
		{"name": "Title"},
		{"name": "Body"},
		{"name": "Amount"}
	]`)

	data := s.SampleTemplateData(context.Background(), vars)

	if _, ok := data["site_name"]; ok {
		t.Error("site_name must not be sampled — worker resolves it")
	}
	if _, ok := data["logo_url"]; ok {
		t.Error("logo_url must not be sampled — worker resolves it")
	}
	if data["user_name"] != "Test User" {
		t.Errorf("user_name = %v", data["user_name"])
	}
	if body, _ := data["body"].(string); body == "" {
		t.Error("body should be sampled")
	}
	if url, _ := data["action_url"].(string); url == "" || url[0] != 'h' {
		t.Errorf("action_url should be an absolute URL, got %q", url)
	}
	if data["amount"] != "25.00" {
		t.Errorf("amount = %v", data["amount"])
	}
}
