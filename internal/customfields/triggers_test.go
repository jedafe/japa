package customfields

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// send_email dispatches the configured subject/body through the wired sender
// with field placeholders expanded and the optional template_slug passed
// through.
func TestFireSendEmailPassesTemplateAndExpandsValues(t *testing.T) {
	var got TriggerEmail
	d := &TriggerDispatcher{
		emailSender: func(_ context.Context, mail TriggerEmail) error {
			got = mail
			return nil
		},
	}

	cfg := map[string]any{
		"to":            "ops@example.com",
		"subject":       "New submission {{form_name}}",
		"body":          "From {{email}}",
		"template_slug": "notification",
		"action_url":    "https://example.com/submissions/{{id}}",
	}
	values := map[string]any{"form_name": "Intake", "email": "u@x.com", "id": "42"}

	if err := d.fireSendEmail(context.Background(), cfg, values); err != nil {
		t.Fatalf("fireSendEmail: %v", err)
	}

	if got.To != "ops@example.com" {
		t.Errorf("To = %q", got.To)
	}
	if got.Subject != "New submission Intake" {
		t.Errorf("Subject = %q", got.Subject)
	}
	if got.Body != "From u@x.com" {
		t.Errorf("Body = %q", got.Body)
	}
	if got.TemplateSlug != "notification" {
		t.Errorf("TemplateSlug = %q", got.TemplateSlug)
	}
	if got.ActionURL != "https://example.com/submissions/42" {
		t.Errorf("ActionURL = %q", got.ActionURL)
	}
}

// 'to' accepts {{field_key}} placeholders (e.g. {{email}} mails the
// submitter); the expanded value must still resolve to a single address.
func TestFireSendEmailToPlaceholder(t *testing.T) {
	var got TriggerEmail
	d := &TriggerDispatcher{
		emailSender: func(_ context.Context, mail TriggerEmail) error {
			got = mail
			return nil
		},
	}

	cfg := map[string]any{"to": "{{email}}", "subject": "s"}
	values := map[string]any{"email": "submitter@example.com"}

	if err := d.fireSendEmail(context.Background(), cfg, values); err != nil {
		t.Fatalf("fireSendEmail: %v", err)
	}
	if got.To != "submitter@example.com" {
		t.Errorf("To = %q, want expanded address", got.To)
	}
}

func TestFireSendEmailToPlaceholderBadValue(t *testing.T) {
	called := false
	d := &TriggerDispatcher{
		emailSender: func(context.Context, TriggerEmail) error { called = true; return nil },
	}

	// The placeholder resolves to a non-address — must error and not send.
	cfg := map[string]any{"to": "{{email}}"}
	values := map[string]any{"email": "not-an-address"}

	if err := d.fireSendEmail(context.Background(), cfg, values); err == nil {
		t.Fatal("expected invalid-address error")
	}
	if called {
		t.Fatal("sender must not fire for an invalid expanded 'to'")
	}
}

func TestFireSendEmailNoSenderSkips(t *testing.T) {
	d := &TriggerDispatcher{}
	if err := d.fireSendEmail(context.Background(), map[string]any{"to": "a@b.com"}, nil); err != nil {
		t.Fatalf("unwired sender should skip, got %v", err)
	}
}

func TestFireSendEmailRequiresTo(t *testing.T) {
	d := &TriggerDispatcher{emailSender: func(context.Context, TriggerEmail) error { return nil }}
	if err := d.fireSendEmail(context.Background(), map[string]any{}, nil); err == nil {
		t.Fatal("missing to should error")
	}
}

// Fire-time slug validation covers rows predating the create/update gate.
func TestFireSendEmailRejectsInvalidTemplateSlug(t *testing.T) {
	called := false
	d := &TriggerDispatcher{
		emailSender: func(context.Context, TriggerEmail) error { called = true; return nil },
	}

	cfg := map[string]any{"to": "a@b.com", "template_slug": "Bad Slug!"}
	if err := d.fireSendEmail(context.Background(), cfg, nil); err == nil {
		t.Fatal("invalid template_slug should error")
	}
	if called {
		t.Fatal("sender must not fire for an invalid slug")
	}
}

func TestValidateTriggerConfigSendEmail(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		wantOK  bool
		wantMsg string
	}{
		{"minimal valid", `{"to":"a@b.com"}`, true, ""},
		{"with template slug", `{"to":"a@b.com","template_slug":"notification"}`, true, ""},
		{"with action url", `{"to":"a@b.com","action_url":"https://x.com/s/1"}`, true, ""},
		{"missing to", `{"subject":"hi"}`, false, "send_email requires config.to"},
		{"to placeholder", `{"to":"{{email}}"}`, true, ""},
		{"bad address", `{"to":"not-an-email"}`, false, "send_email config.to must be a single email address"},
		{"bad slug", `{"to":"a@b.com","template_slug":"Not A Slug"}`, false, "snake_case"},
		{"empty slug ok", `{"to":"a@b.com","template_slug":""}`, true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := validateTriggerConfig("send_email", json.RawMessage(tc.config))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (msg %q)", ok, tc.wantOK, msg)
			}
			if !tc.wantOK && !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("msg %q should contain %q", msg, tc.wantMsg)
			}
		})
	}
}
