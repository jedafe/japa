package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SendPulseProvider implements EmailProvider against the SendPulse SMTP API.
// Auth is OAuth2 client_credentials (POST {base}/oauth/access_token); tokens
// are cached in-process until near expiry. Send is POST {base}/smtp/emails
// with base64-encoded html/text bodies.
// Ref: .jeos/adr/ADR-015-multi-provider-email-abstraction.md
type SendPulseProvider struct {
	name         string
	clientID     string
	clientSecret string
	baseURL      string
	from         string

	mu        sync.Mutex
	token     string
	tokenExpr time.Time
}

// NewSendPulseProvider constructs a SendPulse provider. clientSecret is the
// decrypted secret; baseURL may be empty (defaults to
// https://api.sendpulse.com).
func NewSendPulseProvider(name, clientID, clientSecret, baseURL, from string) *SendPulseProvider {
	if name == "" {
		name = "sendpulse"
	}

	return &SendPulseProvider{
		name:         name,
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      defaultBaseURL(baseURL, "https://api.sendpulse.com"),
		from:         from,
	}
}

// Name returns the provider identifier.
func (p *SendPulseProvider) Name() string { return p.name }

// invalidateToken drops the cached token so the next accessToken call
// fetches a fresh one — used when the API rejects a token before its
// nominal expiry (revoked/rotated server-side).
func (p *SendPulseProvider) invalidateToken() {
	p.mu.Lock()
	p.token = ""
	p.mu.Unlock()
}

// accessToken returns a cached OAuth token, fetching a fresh one when the
// cached token is absent or within 60s of expiry.
func (p *SendPulseProvider) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token != "" && time.Now().Before(p.tokenExpr.Add(-60*time.Second)) {
		return p.token, nil
	}

	_, respBody, err := postJSON(ctx, p.baseURL+"/oauth/access_token", nil, map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     p.clientID,
		"client_secret": p.clientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("sendpulse token: %w", err)
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("sendpulse token: unexpected response")
	}

	p.token = out.AccessToken

	p.tokenExpr = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.ExpiresIn <= 0 {
		p.tokenExpr = time.Now().Add(30 * time.Minute)
	}

	return p.token, nil
}

// Send dispatches an email via SendPulse.
func (p *SendPulseProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := sanitizeAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendpulse send: %w", err)
	}

	from, err := sanitizeAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("sendpulse misconfigured (from_address): %w", err)
	}

	token, err := p.accessToken(ctx)
	if err != nil {
		return EmailResponse{}, err
	}

	mail := map[string]any{
		"subject": sanitizeHeaderValue(req.Subject),
		"from":    map[string]string{"email": from},
		"to":      []map[string]string{{"email": to}},
	}
	if req.IsHTML {
		mail["html"] = base64.StdEncoding.EncodeToString([]byte(req.Body))
	} else {
		mail["text"] = base64.StdEncoding.EncodeToString([]byte(req.Body))
	}

	if len(req.Headers) > 0 {
		hdrs := make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			hdrs[sanitizeHeaderValue(k)] = sanitizeHeaderValue(v)
		}
		mail["headers"] = hdrs
	}

	status, respBody, err := p.sendOnce(ctx, token, mail)
	if status == http.StatusUnauthorized {
		// Cached token rejected before nominal expiry — drop it, fetch a
		// fresh one, and retry once before falling through to the next
		// provider in the chain.
		p.invalidateToken()

		token, err = p.accessToken(ctx)
		if err != nil {
			return EmailResponse{}, err
		}

		status, respBody, err = p.sendOnce(ctx, token, mail)
	}
	if err != nil {
		return EmailResponse{}, err
	}

	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBody, &out)
	_ = status

	return EmailResponse{ProviderRef: out.ID, Status: "success"}, nil
}

// sendOnce performs a single authenticated POST to the SendPulse send
// endpoint, returning the raw status so callers can distinguish auth
// failures from other errors.
func (p *SendPulseProvider) sendOnce(ctx context.Context, token string, mail map[string]any) (int, []byte, error) {
	return postJSON(ctx, p.baseURL+"/smtp/emails", map[string]string{
		"Authorization": "Bearer " + token,
	}, map[string]any{"email": mail})
}
