package federation

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTP Signatures (draft-cavage-11, rsa-sha256) — the de-facto transport
// authentication for ActivityPub delivery. Outbound deliveries (M2) sign
// `(request-target) host date digest`; inbound inbox POSTs are verified
// against the signing actor's publicKeyPem.
//
// Ref: japaconnect-engineering/sections/126-activitypub-federation.md

const signatureAlgorithm = "rsa-sha256"

// SignHeaders lists the covered headers for outbound POSTs in order.
var signHeaders = []string{"(request-target)", "host", "date", "digest"}

// DigestHeader computes the SHA-256 Digest header value for a body.
func DigestHeader(body []byte) string {
	sum := sha256.Sum256(body)
	return "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
}

// RequestTarget returns the `(request-target)` pseudo-header value:
// lowercase method + space + path (+ query when present).
func RequestTarget(r *http.Request) string {
	target := r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	return strings.ToLower(r.Method) + " " + target
}

// signingHeaderValue resolves one covered header name to its canonical
// value for the signing string. params carries the Signature header's
// key="value" parameters — draft-cavage-11 pseudo-headers (created) and
// (expires) resolve to the params of the same name, not HTTP headers.
func signingHeaderValue(r *http.Request, name string, body []byte, params map[string]string) (string, error) {
	switch strings.ToLower(name) {
	case "(request-target)":
		return RequestTarget(r), nil
	case "(created)":
		return params["created"], nil
	case "(expires)":
		return params["expires"], nil
	case "host":
		return r.Host, nil
	case "date":
		return r.Header.Get("Date"), nil
	case "digest":
		d := r.Header.Get("Digest")
		if d == "" && body != nil {
			d = DigestHeader(body)
		}
		return d, nil
	case "content-type":
		return r.Header.Get("Content-Type"), nil
	case "content-length":
		// Go lifts Content-Length off r.Header into r.ContentLength — a
		// signer covering it would otherwise read "" and fail verify.
		if r.ContentLength >= 0 {
			return strconv.FormatInt(r.ContentLength, 10), nil
		}
		return "", nil
	default:
		return r.Header.Get(name), nil
	}
}

// BuildSigningString assembles the covered-header signing text:
// one `name: value` line per header in order, joined by newline. params
// supplies Signature parameters for the (created)/(expires) pseudo-
// headers; pass nil when signing (outbound never covers them).
func BuildSigningString(r *http.Request, headers []string, body []byte, params map[string]string) (string, error) {
	lines := make([]string, 0, len(headers))
	for _, name := range headers {
		v, err := signingHeaderValue(r, name, body, params)
		if err != nil {
			return "", err
		}
		if v == "" && !strings.EqualFold(name, "(request-target)") {
			return "", fmt.Errorf("federation: missing signed header %q", name)
		}
		lines = append(lines, strings.ToLower(name)+": "+v)
	}
	return strings.Join(lines, "\n"), nil
}

// SignRequest sets Date (+ Digest for bodies) and the Signature header on
// an outbound request using draft-cavage rsa-sha256 over signHeaders.
// Date uses UTC RFC1123 (http.TimeFormat) as peers expect.
func SignRequest(r *http.Request, keyID string, priv *rsa.PrivateKey, body []byte) error {
	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	if body != nil {
		r.Header.Set("Digest", DigestHeader(body))
	}

	covered := signHeaders
	if body == nil {
		covered = []string{"(request-target)", "host", "date"}
	}

	signingString, err := BuildSigningString(r, covered, body, nil)
	if err != nil {
		return err
	}

	sum := sha256.Sum256([]byte(signingString))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return fmt.Errorf("federation: sign request: %w", err)
	}

	r.Header.Set("Signature", fmt.Sprintf(
		`keyId=%q,algorithm=%q,headers=%q,signature=%q`,
		keyID, signatureAlgorithm, strings.Join(covered, " "),
		base64.StdEncoding.EncodeToString(sig),
	))
	return nil
}

// ParseSignatureHeader parses the Signature header's comma-separated
// key="value" parameters into a map. Quoted strings may contain commas.
func ParseSignatureHeader(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range splitSignatureParams(raw) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

// splitSignatureParams splits on commas that are not inside quotes.
func splitSignatureParams(raw string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, ch := range raw {
		switch {
		case ch == '"':
			inQuote = !inQuote
			cur.WriteRune(ch)
		case ch == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// signatureClockSkew bounds Date-header staleness for inbound signatures
// (Mastodon convention: 12h). Limits replay of captured signed requests.
const signatureClockSkew = 12 * time.Hour

// VerifySignature verifies an inbound request's Signature header against
// the actor's public key. body is required when the covered headers
// include `digest` (always true for POST deliveries). The `date` header
// must be covered and within signatureClockSkew of now.
func VerifySignature(r *http.Request, pub *rsa.PublicKey, body []byte) error {
	params := ParseSignatureHeader(r.Header.Get("Signature"))
	sigB64 := params["signature"]
	if sigB64 == "" {
		return errors.New("federation: missing signature")
	}

	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("federation: bad signature encoding: %w", err)
	}

	covered := []string{"date"}
	if h := params["headers"]; h != "" {
		covered = strings.Fields(h)
	}

	// Freshness: every covered time bound is enforced as signed —
	// draft-cavage-02 signers cover `date`; draft-cavage-11 signers
	// (Pleroma/Akkoma, Iceshrimp/Sharkey) cover the (created)/(expires)
	// pseudo-headers, whose values are Signature params, not headers. At
	// least one of date/(created) must be covered, else an old signed
	// request replays forever.
	timeBound := false
	for _, name := range covered {
		switch strings.ToLower(name) {
		case "date":
			timeBound = true
			ts, err := http.ParseTime(r.Header.Get("Date"))
			if err != nil {
				return fmt.Errorf("federation: bad date header: %w", err)
			}
			if skew := time.Since(ts); skew > signatureClockSkew || skew < -signatureClockSkew {
				return fmt.Errorf("federation: signature date outside %s skew", signatureClockSkew)
			}
		case "(created)":
			timeBound = true
			created, err := strconv.ParseInt(params["created"], 10, 64)
			if err != nil {
				return errors.New("federation: bad created signature param")
			}
			if skew := time.Since(time.Unix(created, 0)); skew > signatureClockSkew || skew < -signatureClockSkew {
				return fmt.Errorf("federation: signature created outside %s skew", signatureClockSkew)
			}
		case "(expires)":
			expires, err := strconv.ParseInt(params["expires"], 10, 64)
			if err != nil {
				return errors.New("federation: bad expires signature param")
			}
			if time.Now().Unix() > expires {
				return errors.New("federation: signature expired")
			}
		}
	}
	if !timeBound {
		return errors.New("federation: no date or (created) covered by signature")
	}

	// Body integrity: when digest is covered, it must match the payload.
	for _, name := range covered {
		if !strings.EqualFold(name, "digest") {
			continue
		}
		if r.Header.Get("Digest") != DigestHeader(body) {
			return errors.New("federation: digest mismatch")
		}
	}

	signingString, err := BuildSigningString(r, covered, body, params)
	if err != nil {
		return err
	}

	sum := sha256.Sum256([]byte(signingString))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return fmt.Errorf("federation: signature invalid: %w", err)
	}
	return nil
}
