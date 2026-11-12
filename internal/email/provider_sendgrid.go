package email

import (
	"context"
	"fmt"
)

// SendGridProvider implements EmailProvider against the SendGrid v3 API
// (POST {base}/mail/send, Bearer auth).
// Ref: .jeos/adr/ADR-015-multi-provider-email-abstraction.md
type SendGridProvider struct {
	name    string
	apiKey  string
	baseURL string
	from    string
}

// NewSendGridProvider constructs a SendGrid provider. apiKey is the
// decrypted secret; baseURL may be empty (defaults to
// https://api.sendgrid.com/v3).
func NewSendGridProvider(name, apiKey, baseURL, from string) *SendGridProvider {
	if name == "" {
		name = "sendgrid"
	}

	return &SendGridProvider{
		name:    name,
		apiKey:  apiKey,
		baseURL: defaultBaseURL(baseURL, "https://api.sendgrid.com/v3"),
		from:    from,
	}
}

// Name returns the provider identifier.
func (p *SendGridProvider) Name() string { return p.name }

// Send dispatches an email via SendGrid.
func (p *SendGridProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := sanitizeAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendgrid send: %w", err)
	}

	from, err := sanitizeAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendgrid misconfigured (from_address): %w", err)
	}

	mimeType := "text/plain"
	if req.IsHTML {
		mimeType = "text/html"
	}

	body := map[string]any{
		"personalizations": []map[string]any{
			{"to": []map[string]string{{"email": to}}},
		},
		"from":    map[string]string{"email": from},
		"subject": sanitizeHeaderValue(req.Subject),
		"content": []map[string]string{
			{"type": mimeType, "value": req.Body},
		},
	}

	if len(req.Headers) > 0 {
		hdrs := make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			hdrs[sanitizeHeaderValue(k)] = sanitizeHeaderValue(v)
		}

		body["headers"] = hdrs
	}

	_, _, err = postJSON(ctx, p.baseURL+"/mail/send", map[string]string{
		"Authorization": "Bearer " + p.apiKey,
	}, body)
	if err != nil {
		return EmailResponse{}, err
	}

	// SendGrid returns 202 with the message reference in X-Message-Id;
	// postJSON discards headers, so the ref is left empty — the send status
	// is what the dispatcher needs.
	return EmailResponse{Status: "success"}, nil
}
