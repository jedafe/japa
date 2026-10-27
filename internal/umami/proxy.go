// Package umami proxies read-only analytics requests from the Control
// Panel to the self-hosted Umami instance. Umami v2.20 has no API-key
// support, so the proxy authenticates with a service-account login and
// caches the session token, refreshing it on expiry.
//
// Config: JC_UMAMI_URL, JC_UMAMI_USERNAME, JC_UMAMI_PASSWORD.
package umami

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Proxy forwards allowlisted GET paths to the Umami REST API.
type Proxy struct {
	base   string
	user   string
	pass   string
	client *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewProxy builds a proxy from JC_UMAMI_* environment variables.
func NewProxy() *Proxy {
	return &Proxy{
		base:   strings.TrimSuffix(os.Getenv("JC_UMAMI_URL"), "/"),
		user:   os.Getenv("JC_UMAMI_USERNAME"),
		pass:   os.Getenv("JC_UMAMI_PASSWORD"),
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled reports whether the proxy is configured.
func (p *Proxy) Enabled() bool { return p.base != "" && p.user != "" && p.pass != "" }

// allowedSubpath restricts the proxy to read-only website reporting
// endpoints — no auth, admin, or mutation surface is forwarded.
var allowedSubpath = regexp.MustCompile(`^websites(/[0-9a-fA-F-]{36}/(stats|pageviews|metrics|events|active|reports)(/series)?)?$`)

// ServeHTTP handles GET /api/v1/analytics/umami/* — the wildcard is the
// Umami API path under /api/, query string forwarded verbatim.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.Enabled() {
		http.Error(w, `{"error":"umami not configured"}`, http.StatusServiceUnavailable)
		return
	}
	sub := chi.URLParam(r, "*")
	if !allowedSubpath.MatchString(sub) {
		http.Error(w, `{"error":"path not allowed"}`, http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	status, body, ct, err := p.forward(r.Context(), sub, r.URL.RawQuery, false)
	if err == errUnauthorized {
		status, body, ct, err = p.forward(r.Context(), sub, r.URL.RawQuery, true)
	}
	if err != nil {
		http.Error(w, `{"error":"umami upstream error"}`, http.StatusBadGateway)
		return
	}
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

var errUnauthorized = fmt.Errorf("unauthorized")

func (p *Proxy) forward(ctx context.Context, sub, rawq string, relogin bool) (int, []byte, string, error) {
	tok, err := p.getToken(ctx, relogin)
	if err != nil {
		return 0, nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/"+sub+"?"+rawq, nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := p.client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return 0, nil, "", err
	}
	if res.StatusCode == http.StatusUnauthorized && !relogin {
		return 0, nil, "", errUnauthorized
	}
	return res.StatusCode, body, res.Header.Get("Content-Type"), nil
}

func (p *Proxy) getToken(ctx context.Context, refresh bool) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !refresh && p.token != "" && time.Now().Before(p.expiry) {
		return p.token, nil
	}
	payload, _ := json.Marshal(map[string]string{"username": p.user, "password": p.pass})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/api/auth/login", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("umami login: status %d", res.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil || out.Token == "" {
		return "", fmt.Errorf("umami login: bad response")
	}
	p.token, p.expiry = out.Token, time.Now().Add(6*time.Hour)
	return p.token, nil
}
