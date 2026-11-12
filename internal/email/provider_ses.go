package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SESProvider implements EmailProvider against the Amazon SES v2 REST API
// (POST https://email.{region}.amazonaws.com/v2/email/outbound-emails).
// Requests are signed with AWS Signature Version 4 — no AWS SDK dependency.
// The message is sent as Raw MIME so custom headers (List-Unsubscribe etc.)
// are preserved, matching the SMTP provider's capability.
// Ref: .jeos/adr/ADR-015-multi-provider-email-abstraction.md
type SESProvider struct {
	name    string
	baseURL string
	region  string
	akid    string
	secret  string
	from    string
}

// NewSESProvider constructs an SES provider. secret is the decrypted AWS
// secret access key; baseURL may be empty (defaults to the regional
// endpoint derived from region).
func NewSESProvider(name, region, accessKeyID, secret, baseURL, from string) *SESProvider {
	if name == "" {
		name = "ses"
	}

	if region == "" {
		region = "us-east-1"
	}

	if baseURL == "" {
		baseURL = "https://email." + region + ".amazonaws.com"
	}

	return &SESProvider{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		region:  region,
		akid:    accessKeyID,
		secret:  secret,
		from:    from,
	}
}

// Name returns the provider identifier.
func (p *SESProvider) Name() string { return p.name }

// Send dispatches an email via SES SendEmail (Raw content).
func (p *SESProvider) Send(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	to, err := sanitizeAddress(req.To)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("ses send: %w", err)
	}

	from, err := sanitizeAddress(p.from)
	if err != nil {
		return EmailResponse{}, fmt.Errorf("ses misconfigured (from_address): %w", err)
	}

	mimeMsg := buildMIMEMessage(from, to, req.Subject, req.Body, req.IsHTML, req.Headers)

	payload, err := json.Marshal(map[string]any{
		"FromEmailAddress": from,
		"Destination":      map[string]any{"ToAddresses": []string{to}},
		"Content": map[string]any{
			"Raw": map[string]any{"Data": mimeMsg},
		},
	})
	if err != nil {
		return EmailResponse{}, fmt.Errorf("ses marshal: %w", err)
	}

	url := p.baseURL + "/v2/email/outbound-emails"
	if err := validateEmailEndpoint(url); err != nil {
		return EmailResponse{}, err
	}

	status, respBody, err := p.signedPost(ctx, url, payload)
	if err != nil {
		return EmailResponse{}, err
	}

	var out struct {
		MessageID string `json:"MessageId"`
	}
	_ = json.Unmarshal(respBody, &out)
	_ = status

	return EmailResponse{ProviderRef: out.MessageID, Status: "success"}, nil
}

// signedPost signs payload with SigV4 and POSTs it, returning the HTTP
// status and response body. Non-2xx responses are errors so the dispatcher
// falls through to the next provider.
func (p *SESProvider) signedPost(ctx context.Context, url string, payload []byte) (int, []byte, error) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	payloadHashBytes := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	host := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}

	canonicalURI := "/v2/email/outbound-emails"
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"

	canonicalHeaders := "content-type:application/json\n" +
		"host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"

	canonicalRequest := "POST\n" + canonicalURI + "\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash

	credentialScope := dateStamp + "/" + p.region + "/ses/aws4_request"
	crHashBytes := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + hex.EncodeToString(crHashBytes[:])

	signature := hex.EncodeToString(sesV4Signature(p.secret, dateStamp, p.region, stringToSign))

	authHeader := "AWS4-HMAC-SHA256 Credential=" + p.akid + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	req, err := newSignedRequest(ctx, url, payload)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Authorization", authHeader)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("ses request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("ses read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, body, fmt.Errorf("ses returned %d: %s", resp.StatusCode, truncateForLog(string(body)))
	}

	return resp.StatusCode, body, nil
}

// sesV4Signature derives the SigV4 signing key and returns the signature
// for stringToSign. Chain: HMAC("AWS4"+secret, date) → region → service →
// "aws4_request", then HMAC(key, stringToSign).
func sesV4Signature(secret, dateStamp, region, stringToSign string) []byte {
	h := func(key, data []byte) []byte {
		m := hmac.New(sha256.New, key)
		m.Write(data)
		return m.Sum(nil)
	}

	kDate := h([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := h(kDate, []byte(region))
	kService := h(kRegion, []byte("ses"))
	kSigning := h(kService, []byte("aws4_request"))

	return h(kSigning, []byte(stringToSign))
}

// newSignedRequest builds the HTTP request after endpoint validation.
func newSignedRequest(ctx context.Context, url string, payload []byte) (*http.Request, error) {
	if err := validateEmailEndpoint(url); err != nil {
		return nil, err
	}

	return http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload)) // #nosec G704 -- url validated above
}

// buildMIMEMessage renders an RFC 5322 message identical in shape to the
// SMTP provider's output: sanitized headers, deterministic extra-header
// ordering, and the raw body as content.
func buildMIMEMessage(from, to, subject, body string, isHTML bool, extra map[string]string) []byte {
	contentType := "text/plain; charset=UTF-8"
	if isHTML {
		contentType = "text/html; charset=UTF-8"
	}

	var hdr strings.Builder

	hdr.WriteString("From: " + from + "\r\n")
	hdr.WriteString("To: " + to + "\r\n")
	hdr.WriteString("Subject: " + sanitizeHeaderValue(subject) + "\r\n")
	hdr.WriteString("MIME-Version: 1.0\r\n")
	hdr.WriteString("Content-Type: " + contentType + "\r\n")

	if len(extra) > 0 {
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			name := sanitizeHeaderValue(k)
			if name == "" || strings.ContainsAny(name, " :") {
				continue
			}

			hdr.WriteString(name + ": " + sanitizeHeaderValue(extra[k]) + "\r\n")
		}
	}

	return []byte(hdr.String() + "\r\n" + body)
}
