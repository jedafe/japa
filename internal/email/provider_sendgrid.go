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
	to, err := parseAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendgrid send: %w", err)
	}

	from, err := parseAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendgrid misconfigured (from_address): %w", err)
	}

	// SendGrid splits the mailbox into email/name fields — a display name
	// configured as "Name <addr>" in from_address reaches the sender name.
	toField := map[string]string{"email": to.Address}
	if to.Name != "" {
		toField["name"] = to.Name
	}

	fromField := map[string]string{"email": from.Address}
	if from.Name != "" {
		fromField["name"] = from.Name
	}

	// The plain part must precede the HTML part — mail readers render the
	// last content type they support.
	var content []map[string]string
	if req.IsHTML {
		content = []map[string]string{
			{"type": "text/plain", "value": req.TextAlternative()},
			{"type": "text/html", "value": req.Body},
		}
	} else {
		content = []map[string]string{{"type": "text/plain", "value": req.Body}}
	}

	body := map[string]any{
		"personalizations": []map[string]any{
			{"to": []map[string]string{toField}},
		},
		"from":    fromField,
		"subject": sanitizeHeaderValue(req.Subject),
		"content": content,
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
