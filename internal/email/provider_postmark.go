package email

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// PostmarkProvider implements EmailProvider against the Postmark REST API
// (POST {base}/email, X-Postmark-Server-Token auth). The message_stream
// config selects the Postmark stream ("outbound" transactional default;
// a broadcast stream for marketing sends).
// Ref: .jeos/adr/ADR-015-multi-provider-email-abstraction.md
type PostmarkProvider struct {
	name          string
	token         string
	baseURL       string
	from          string
	messageStream string
}

// NewPostmarkProvider constructs a Postmark provider. token is the
// decrypted server token; baseURL may be empty (defaults to
// https://api.postmarkapp.com).
func NewPostmarkProvider(name, token, baseURL, from, messageStream string) *PostmarkProvider {
	if name == "" {
		name = "postmark"
	}

	if messageStream == "" {
		messageStream = "outbound"
	}

	return &PostmarkProvider{
		name:          name,
		token:         token,
		baseURL:       defaultBaseURL(baseURL, "https://api.postmarkapp.com"),
		from:          from,
		messageStream: messageStream,
	}
}

// Name returns the provider identifier.
func (p *PostmarkProvider) Name() string { return p.name }

// Send dispatches an email via Postmark.
func (p *PostmarkProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := sanitizeAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("postmark send: %w", err)
	}

	from, err := sanitizeAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("postmark misconfigured (from_address): %w", err)
	}

	body := map[string]any{
		"From":          from,
		"To":            to,
		"Subject":       sanitizeHeaderValue(req.Subject),
		"MessageStream": p.messageStream,
	}
	if req.IsHTML {
		body["HtmlBody"] = req.Body
	} else {
		body["TextBody"] = req.Body
	}

	if len(req.Headers) > 0 {
		keys := make([]string, 0, len(req.Headers))
		for k := range req.Headers {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		hdrs := make([]map[string]string, 0, len(keys))
		for _, k := range keys {
			hdrs = append(hdrs, map[string]string{
				"Name":  sanitizeHeaderValue(k),
				"Value": sanitizeHeaderValue(req.Headers[k]),
			})
		}

		body["Headers"] = hdrs
	}

	_, respBody, err := postJSON(ctx, p.baseURL+"/email", map[string]string{
		"X-Postmark-Server-Token": p.token,
		"Accept":                  "application/json",
	}, body)
	if err != nil {
		return EmailResponse{}, err
	}

	var out struct {
		MessageID string `json:"MessageID"`
	}
	_ = json.Unmarshal(respBody, &out)

	return EmailResponse{ProviderRef: out.MessageID, Status: "success"}, nil
}
