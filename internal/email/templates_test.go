package email

import (
	"strings"
	"testing"
)

func sampleData() TemplateData {
	return TemplateData{
		SiteName:   "JapaConnect",
		SiteURL:    "https://japaconnect.test",
		UserName:   "Ada",
		ActionURL:  "https://japaconnect.test/action?token=abc123",
		Expiry:     "1 hour",
		SupportURL: "https://japaconnect.test/support",
	}
}

func TestRenderTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		render func(TemplateData) (string, error)
		title  string
	}{
		{name: "password reset", render: RenderPasswordResetEmail, title: "Reset Your Password"},
		{name: "email verification", render: RenderEmailVerification, title: "Verify Your Email"},
		{name: "welcome", render: RenderWelcomeEmail, title: "Welcome"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := sampleData()

			body, err := tt.render(data)
			if err != nil {
				t.Fatalf("render: %v", err)
			}

			if !strings.Contains(body, tt.title) {
				t.Errorf("body missing title %q", tt.title)
			}

			for _, want := range []string{data.SiteName, data.UserName, data.SupportURL} {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q", want)
				}
			}

			if strings.Contains(body, "{{") {
				t.Error("body contains an unrendered template action")
			}
		})
	}
}

func TestRenderPasswordResetIncludesActionAndExpiry(t *testing.T) {
	t.Parallel()

	data := sampleData()

	body, err := RenderPasswordResetEmail(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// html/template escapes & in URLs, so compare against the escaped form.
	if !strings.Contains(body, strings.ReplaceAll(data.ActionURL, "&", "&amp;")) {
		t.Error("body missing action URL")
	}

	if !strings.Contains(body, data.Expiry) {
		t.Errorf("body missing expiry %q", data.Expiry)
	}
}

func TestRenderOmitsGreetingWithoutUserName(t *testing.T) {
	t.Parallel()

	data := sampleData()
	data.UserName = ""

	body, err := RenderPasswordResetEmail(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(body, "Hi ,") {
		t.Error("body contains an empty greeting")
	}
}

func TestRenderEscapesUserSuppliedMarkup(t *testing.T) {
	t.Parallel()

	data := sampleData()
	data.UserName = `<script>alert("xss")</script>`

	body, err := RenderWelcomeEmail(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(body, "<script>") {
		t.Error("user-supplied markup was not escaped")
	}

	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("escaped user name not present in body")
	}
}
