package email

import (
	"context"
	"encoding/json"
	"fmt"
)

// ResendProvider implements EmailProvider against the Resend REST API
// (POST {base}/emails, Bearer auth).
// Ref: .jeos/adr/ADR-015-multi-provider-email-abstraction.md
type ResendProvider struct {
	name    string
	apiKey  string
	baseURL string
	from    string
}

// NewResendProvider constructs a Resend provider. apiKey is the decrypted
// secret; baseURL may be empty (defaults to https://api.resend.com).
func NewResendProvider(name, apiKey, baseURL, from string) *ResendProvider {
	if name == "" {
		name = "resend"
	}

	return &ResendProvider{
		name:    name,
		apiKey:  apiKey,
		baseURL: defaultBaseURL(baseURL, "https://api.resend.com"),
		from:    from,
	}
}

// Name returns the provider identifier.
func (p *ResendProvider) Name() string { return p.name }

// Send dispatches an email via Resend.
func (p *ResendProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := parseAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("resend send: %w", err)
	}

	from, err := parseAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("resend misconfigured (from_address): %w", err)
	}

	// Resend accepts the RFC 5322 "Name <addr>" form in from/to — pass the
	// display-name-preserving string so configured sender names survive.
	body := map[string]any{
		"from":    headerAddress(from),
		"to":      []string{headerAddress(to)},
		"subject": sanitizeHeaderValue(req.Subject),
	}
	if req.IsHTML {
		body["html"] = req.Body
		body["text"] = req.TextAlternative()
	} else {
		body["text"] = req.Body
	}

	if len(req.Headers) > 0 {
		hdrs := make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			hdrs[sanitizeHeaderValue(k)] = sanitizeHeaderValue(v)
		}
		body["headers"] = hdrs
	}

	_, respBody, err := postJSON(ctx, p.baseURL+"/emails", map[string]string{
		"Authorization": "Bearer " + p.apiKey,
	}, body)
	if err != nil {
		return EmailResponse{}, err
	}

	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &out)

	return EmailResponse{ProviderRef: out.ID, Status: "success"}, nil
}
